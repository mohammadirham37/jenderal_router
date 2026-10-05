// Package router memecahkan nama model/alias/combo menjadi langkah eksekusi
// dan menangani fallback antar langkah, retry credential, serta circuit
// breaker per provider (F-06, F-14; FR-2.1–2.7).
package router

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jenderal/jenderalrouter/internal/translate"
)

// Step satu langkah combo.
type Step struct {
	ModelID    int64
	ProviderID int64
	PublicID   string // untuk header X-Route-Model
	Alias      string // nama tampilan/model API; kosong = pakai PublicID
	Upstream   string // nama model upstream
	Format     translate.Format
	TimeoutMs  int
}

// Display nama model yang ditampilkan ke klien: alias bila diisi, else public_id.
func (s Step) Display() string {
	if s.Alias != "" {
		return s.Alias
	}
	return s.PublicID
}

// Resolver mengubah nama (alias / public_id / nama combo) menjadi langkah.
type Resolver interface {
	Resolve(name string) ([]Step, error)
}

// UpstreamError dipinjam dari translate agar classifier bekerja langsung.
type UpstreamError = translate.UpstreamError

// ErrNoStep ketika combo kosong.
var ErrNoStep = errors.New("combo tidak memiliki langkah")

// CircuitBreaker per provider: >=5 kegagalan dalam 60 detik → open 30 detik
// → half-open (FR-2.5).
type CircuitBreaker struct {
	mu           sync.Mutex
	failures     []time.Time // waktu kegagalan dalam window
	openedAt     time.Time
	halfOpen     bool
	failureLimit int
	window       time.Duration
	cooldown     time.Duration
}

// NewBreaker membuat breaker dengan parameter standar FR-2.5.
func NewBreaker() *CircuitBreaker {
	return &CircuitBreaker{failureLimit: 5, window: 60 * time.Second, cooldown: 30 * time.Second}
}

// Allow melaporkan apakah provider boleh dicoba sekarang.
func (b *CircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.prune(now)
	if b.openedAt.IsZero() {
		return true
	}
	if now.Sub(b.openedAt) >= b.cooldown {
		// half-open: izinkan satu percobaan
		b.halfOpen = true
		return true
	}
	return false
}

// RecordSuccess mencatat keberhasilan (menutup breaker).
func (b *CircuitBreaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = nil
	b.openedAt = time.Time{}
	b.halfOpen = false
}

// RecordFailure mencatat kegagalan; mengembalikan apakah breaker kini open.
func (b *CircuitBreaker) RecordFailure() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.prune(now)
	b.failures = append(b.failures, now)
	if b.halfOpen || len(b.failures) >= b.failureLimit {
		b.openedAt = now
		b.halfOpen = false
		return true
	}
	return false
}

// IsOpen status breaker saat ini.
func (b *CircuitBreaker) IsOpen() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openedAt.IsZero() {
		return false
	}
	if time.Since(b.openedAt) >= b.cooldown {
		return false // half-open dianggap boleh
	}
	return true
}

func (b *CircuitBreaker) prune(now time.Time) {
	kept := b.failures[:0]
	for _, t := range b.failures {
		if now.Sub(t) < b.window {
			kept = append(kept, t)
		}
	}
	b.failures = kept
}

// Breakers breaker per provider.
type Breakers struct {
	mu       sync.Mutex
	breakers map[int64]*CircuitBreaker
}

// NewBreakers membuat registry breaker.
func NewBreakers() *Breakers {
	return &Breakers{breakers: map[int64]*CircuitBreaker{}}
}

// For mengambil breaker provider (membuat bila belum ada).
func (bs *Breakers) For(providerID int64) *CircuitBreaker {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	b, ok := bs.breakers[providerID]
	if !ok {
		b = NewBreaker()
		bs.breakers[providerID] = b
	}
	return b
}

// AttemptData statistik satu percobaan upstream (untuk log & error).
type AttemptData struct {
	StepIndex    int
	ProviderID   int64
	ProviderName string
	ModelPublic  string
	CredentialID int64
	Err          error
	LatencyMs    int64
}

// AttemptResult hasil satu percobaan yang berhasil.
type AttemptResult struct {
	StepIndex    int
	ProviderID   int64
	ProviderName string
	ModelPublic  string
	ModelDisplay string // alias tampilan (fallback ModelPublic)
	CredentialID int64
	Stream       StreamHandle
	Response     *translate.ChatResponse
}

// StreamHandle membungkus event stream adapter untuk handler.
type StreamHandle interface {
	Next() (translate.Event, bool)
	Close()
}

// Sender fungsi eksekusi satu langkah (disediakan oleh orchestrator inferensi).
// Harus hanya mengembalikan error yang layak fallback (FR-2.2 diputuskan caller).
type Sender func(ctx context.Context, step Step) (*AttemptResult, error)

// Runner mengeksekusi langkah combo dengan fallback/retry (FR-2.2–2.4).
type Runner struct {
	Breakers *Breakers
	Sender   Sender
	// MaxRetrySameProvider: 1 retry ke credential lain (FR-2.4)
	MaxRetrySameProvider int
}

// Run menjalankan seluruh langkah; mengembalikan percobaan pertama yang sukses.
// Semua percobaan harus terjadi sebelum token pertama (FR-2.3 dijaga handler:
// sender streaming dipanggil di sini sebelum apa pun dikirim ke klien).
func (r *Runner) Run(ctx context.Context, steps []Step) (*AttemptResult, []AttemptData, error) {
	var attempts []AttemptData
	if len(steps) == 0 {
		return nil, attempts, ErrNoStep
	}
	retried := map[int64]bool{} // provider yang sudah di-retry sekali
	for i, step := range steps {
		// circuit breaker per provider
		breaker := r.Breakers.For(step.ProviderID)
		if !breaker.Allow() {
			attempts = append(attempts, AttemptData{StepIndex: i, ProviderID: step.ProviderID,
				ModelPublic: step.PublicID, Err: errors.New("circuit breaker terbuka")})
			continue
		}
		maxTries := 1 + r.MaxRetrySameProvider
		var lastErr error
		for try := 0; try < maxTries; try++ {
			res, err := r.Sender(ctx, step)
			if err == nil {
				breaker.RecordSuccess()
				return res, attempts, nil
			}
			lastErr = err
			attempts = append(attempts, AttemptData{StepIndex: i, ProviderID: step.ProviderID,
				ProviderName: step.PublicID, ModelPublic: step.PublicID, Err: err})
			opened := breaker.RecordFailure()
			slog.Warn("langkah gagal", "step", i, "model", step.PublicID, "err", err, "breaker_open", opened)
			// Error yang tidak layak fallback (400, penolakan konten) menghentikan
			// seluruh rantai — mencoba provider lain tidak akan menolong (FR-2.2).
			var ue *UpstreamError
			if !errors.As(err, &ue) || !ue.Retryable() {
				return nil, attempts, err
			}
			if ctx.Err() != nil {
				return nil, attempts, ctx.Err()
			}
		}
		_ = retried
		_ = lastErr
	}
	if len(attempts) > 0 {
		return nil, attempts, attempts[len(attempts)-1].Err
	}
	return nil, attempts, errors.New("semua langkah combo gagal")
}
