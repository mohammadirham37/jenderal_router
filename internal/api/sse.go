package api

// Keep-alive SSE untuk proksi dengan batas idle — Cloudflare Tunnel
// (keepAliveTimeout ~90 dtk) dan edge Cloudflare (~100 dtk byte pertama),
// nginx, dsb. Komentar ": ping" diabaikan semua klien SSE sesuai spesifikasi
// tapi menjaga koneksi tetap hidup selama cold start / fase berpikir model.

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

type sseKeepAlive struct {
	w    http.ResponseWriter
	f    http.Flusher
	mu   *sync.Mutex
	stop chan struct{}
	done sync.WaitGroup
}

// startSSEKeepAlive memulai ping berkala; semua tulisan ke w dari goroutine
// lain wajib lewat mutex yang sama agar tidak tercampur.
func startSSEKeepAlive(w http.ResponseWriter, mu *sync.Mutex, interval time.Duration) *sseKeepAlive {
	f, _ := w.(http.Flusher)
	k := &sseKeepAlive{w: w, f: f, mu: mu, stop: make(chan struct{})}
	k.done.Add(1)
	go func() {
		defer k.done.Done()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-k.stop:
				return
			case <-t.C:
				k.mu.Lock()
				fmt.Fprint(k.w, ": ping\n\n")
				if k.f != nil {
					k.f.Flush()
				}
				k.mu.Unlock()
			}
		}
	}()
	return k
}

// Stop menghentikan ping dan menunggu goroutine selesai — panggil sebelum
// handler kembali agar tidak menulis ke ResponseWriter yang sudah mati.
func (k *sseKeepAlive) Stop() {
	close(k.stop)
	k.done.Wait()
}
