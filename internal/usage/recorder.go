// Package usage mencatat log request dan pemakaian kuota secara asinkron
// (NFR-10): kegagalan pencatatan tidak boleh menggagalkan request.
package usage

import (
	"log/slog"
	"sync"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
)

// Entry satu catatan request selesai.
type Entry struct {
	TS              time.Time
	UserID          int64
	KeyID           int64
	RequestedModel  string
	ProviderID      int64
	ModelID         int64
	ProviderName    string
	ModelName       string
	Status          int
	ErrorCode       string
	LatencyMs       int64
	TTFTMs          int64
	TokensIn        int
	TokensOut       int
	TokensEstimated bool
	CostUSD         float64
	Attempts        int
	Stream          bool
	IP              string
	// Opsional: isi debug hanya bila JR_LOG_PROMPTS aktif (NFR-14)
	ContentDebug string
	// Kuota yang dikurangi bersamaan (bisa lebih dari satu scope)
	QuotaTargets []QuotaTarget
}

// QuotaTarget pasangan scope kuota.
type QuotaTarget struct {
	Scope   string // user|key
	ScopeID int64
}

// Recorder memproses Entry lewat worker batch latar.
type Recorder struct {
	store      *store.Store
	ch         chan Entry
	wg         sync.WaitGroup
	batchSize  int
	flushEvery time.Duration
	dropped    int64
	mu         sync.Mutex
	closeOnce  sync.Once
}

// NewRecorder memulai worker.
func NewRecorder(st *store.Store, queueSize int) *Recorder {
	if queueSize <= 0 {
		queueSize = 4096
	}
	r := &Recorder{store: st, ch: make(chan Entry, queueSize), batchSize: 50, flushEvery: 500 * time.Millisecond}
	r.wg.Add(1)
	go r.worker()
	return r
}

// Record mengantre catatan; tidak pernah blokir (drop + hitung bila penuh).
func (r *Recorder) Record(e Entry) {
	select {
	case r.ch <- e:
	default:
		r.mu.Lock()
		r.dropped++
		r.mu.Unlock()
		slog.Warn("antrean usage penuh; catatan dibuang")
	}
}

// Close menghentikan worker setelah antrean kosong; aman dipanggil dua kali.
func (r *Recorder) Close() {
	r.closeOnce.Do(func() {
		close(r.ch)
	})
	r.wg.Wait()
}

// Dropped jumlah catatan yang dibuang karena antrean penuh.
func (r *Recorder) Dropped() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}

func (r *Recorder) worker() {
	defer r.wg.Done()
	batch := make([]Entry, 0, r.batchSize)
	ticker := time.NewTicker(r.flushEvery)
	defer ticker.Stop()
	flush := func() {
		if len(batch) == 0 {
			return
		}
		r.persist(batch)
		batch = batch[:0]
	}
	for {
		select {
		case e, ok := <-r.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, e)
			if len(batch) >= r.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// persist menulis log + memperbarui kuota; error hanya dilog.
func (r *Recorder) persist(batch []Entry) {
	for _, e := range batch {
		var providerID, modelID any
		if e.ProviderID > 0 {
			providerID = e.ProviderID
		}
		if e.ModelID > 0 {
			modelID = e.ModelID
		}
		var userID, keyID any
		if e.UserID > 0 {
			userID = e.UserID
		}
		if e.KeyID > 0 {
			keyID = e.KeyID
		}
		estimated := 0
		if e.TokensEstimated {
			estimated = 1
		}
		stream := 0
		if e.Stream {
			stream = 1
		}
		_, err := r.store.DB.Exec(`INSERT INTO request_logs
			(ts, user_id, key_id, requested_model, provider_id, model_id, provider_name, model_name,
			 status, error_code, latency_ms, ttft_ms, tokens_in, tokens_out, tokens_estimated,
			 cost_usd, attempts, stream, ip, content_debug)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			e.TS.UTC().Format(time.RFC3339Nano), userID, keyID, e.RequestedModel, providerID, modelID,
			e.ProviderName, e.ModelName, e.Status, e.ErrorCode, e.LatencyMs, e.TTFTMs,
			e.TokensIn, e.TokensOut, estimated, e.CostUSD, e.Attempts, stream, e.IP, e.ContentDebug)
		if err != nil {
			slog.Error("menulis request_log gagal", "err", err)
		}
		for _, qt := range e.QuotaTargets {
			tokens := int64(e.TokensIn + e.TokensOut)
			if err := r.store.RecordQuotaUsage(qt.Scope, qt.ScopeID, tokens, e.CostUSD); err != nil {
				slog.Error("memperbarui kuota gagal", "err", err)
			}
		}
	}
}

// CostUSDCalculate menghitung biaya USD dari token dan harga per 1M (FR-6.6:
// model lokal berharga 0 → biaya 0, token tetap dicatat).
func CostUSDCalculate(tokensIn, tokensOut int, priceInPer1M, priceOutPer1M float64) float64 {
	return float64(tokensIn)/1_000_000*priceInPer1M + float64(tokensOut)/1_000_000*priceOutPer1M
}
