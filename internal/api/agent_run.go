package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// Run agen chat: berjalan di server terlepas dari koneksi browser. Setiap
// kejadian (token, tool, selesai) dicatat berurutan di memori sehingga
// klien bisa menyambung ulang dari urutan mana pun (?from=N) — misalnya
// setelah tab ditutup lalu percakapan dibuka lagi. Run yang selesai
// disimpan sebentar untuk pelanggan yang terlambat, lalu dibuang.

const (
	runMaxDuration = 20 * time.Minute // batas total satu run
	runRetention   = 10 * time.Minute // run selesai tetap bisa diputar ulang
)

type chatRun struct {
	ID     string
	ConvID int64
	UserID int64
	cancel context.CancelFunc

	mu       sync.Mutex
	events   []json.RawMessage
	notify   chan struct{} // ditutup & diganti setiap ada event baru
	done     bool
	finished time.Time
}

// emit menambah event (aman dipanggil dari banyak goroutine).
func (r *chatRun) emit(ev map[string]any) {
	b, _ := json.Marshal(ev)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return
	}
	r.events = append(r.events, b)
	close(r.notify)
	r.notify = make(chan struct{})
}

// finish menandai run selesai (event terakhir sudah di-emit).
func (r *chatRun) finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return
	}
	r.done = true
	r.finished = time.Now()
	close(r.notify)
}

// snapshot event sejak indeks from, status selesai, dan kanal tunggu berikutnya.
func (r *chatRun) snapshot(from int) ([]json.RawMessage, bool, <-chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var evs []json.RawMessage
	if from < len(r.events) {
		evs = append(evs, r.events[from:]...)
	}
	return evs, r.done, r.notify
}

func (r *chatRun) isDone() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.done
}

type runManager struct {
	mu     sync.Mutex
	runs   map[string]*chatRun
	byConv map[int64]*chatRun
	once   sync.Once
}

func newRunManager() *runManager {
	return &runManager{runs: map[string]*chatRun{}, byConv: map[int64]*chatRun{}}
}

// start mendaftarkan run baru; false bila percakapan masih punya run aktif.
func (m *runManager) start(convID, userID int64) (*chatRun, context.Context, bool) {
	m.once.Do(func() { go m.janitor() })
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur := m.byConv[convID]; cur != nil && !cur.isDone() {
		return nil, nil, false
	}
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	ctx, cancel := context.WithTimeout(context.Background(), runMaxDuration)
	r := &chatRun{ID: hex.EncodeToString(b), ConvID: convID, UserID: userID, cancel: cancel, notify: make(chan struct{})}
	m.runs[r.ID] = r
	m.byConv[convID] = r
	return r, ctx, true
}

func (m *runManager) get(id string) *chatRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[id]
}

// active run yang sedang berjalan untuk percakapan (nil bila tidak ada).
func (m *runManager) active(convID int64) *chatRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r := m.byConv[convID]; r != nil && !r.isDone() {
		return r
	}
	return nil
}

func (m *runManager) janitor() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		m.sweep(time.Now())
	}
}

func (m *runManager) sweep(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, r := range m.runs {
		r.mu.Lock()
		expired := r.done && now.Sub(r.finished) > runRetention
		r.mu.Unlock()
		if expired {
			delete(m.runs, id)
			if m.byConv[r.ConvID] == r {
				delete(m.byConv, r.ConvID)
			}
		}
	}
}
