package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jenderal/jenderalrouter/internal/translate"
)

func step(id, provider int64) Step {
	return Step{ModelID: id, ProviderID: provider, PublicID: "p/m", Upstream: "m", Format: translate.FormatOpenAI, TimeoutMs: 5000}
}

func TestBreakerStates(t *testing.T) {
	b := NewBreaker()
	if !b.Allow() {
		t.Fatal("breaker baru harus mengizinkan")
	}
	for i := 0; i < 5; i++ {
		if b.RecordFailure() != (i == 4) {
			t.Logf("iterasi %d open=%v", i, b.RecordFailure())
		}
	}
	if !b.IsOpen() {
		t.Fatal("5 kegagalan harus membuka breaker")
	}
	if b.Allow() {
		t.Fatal("breaker open harus menolak")
	}
	// simulasi lewat cooldown 30 detik
	b.openedAt = time.Now().Add(-31 * time.Second)
	if !b.Allow() {
		t.Fatal("setelah cooldown harus half-open dan mengizinkan")
	}
	// kegagalan di half-open → open lagi
	if !b.RecordFailure() {
		t.Fatal("kegagalan half-open harus membuka")
	}
	// sukses → tertutup
	b.openedAt = time.Now().Add(-31 * time.Second)
	b.Allow()
	b.RecordSuccess()
	if b.IsOpen() {
		t.Fatal("sukses harus menutup breaker")
	}
}

func TestBreakerWindowPrune(t *testing.T) {
	b := NewBreaker()
	// 4 kegagalan lama (di luar window) tidak boleh membuka
	for i := 0; i < 4; i++ {
		b.failures = append(b.failures, time.Now().Add(-2*time.Minute))
	}
	if b.RecordFailure() {
		t.Fatal("kegagalan lama harus dipangkas")
	}
}

func TestRunnerSuccessFirstStep(t *testing.T) {
	r := &Runner{Breakers: NewBreakers(), MaxRetrySameProvider: 1}
	calls := 0
	r.Sender = func(ctx context.Context, s Step) (*AttemptResult, error) {
		calls++
		return &AttemptResult{StepIndex: 0, Response: &translate.ChatResponse{Content: "ok"}}, nil
	}
	res, attempts, err := r.Run(context.Background(), []Step{step(1, 10)})
	if err != nil || res == nil || calls != 1 || len(attempts) != 0 {
		t.Fatalf("res=%v err=%v calls=%d attempts=%d", res, err, calls, len(attempts))
	}
}

func TestRunnerFallbackOnRetryable(t *testing.T) {
	r := &Runner{Breakers: NewBreakers(), MaxRetrySameProvider: 1}
	var log []int // model id per panggilan
	r.Sender = func(ctx context.Context, s Step) (*AttemptResult, error) {
		log = append(log, int(s.ModelID))
		if s.ModelID == 1 {
			return nil, &translate.UpstreamError{StatusCode: 429, Message: "rate limit"}
		}
		return &AttemptResult{StepIndex: 1, Response: &translate.ChatResponse{Content: "ok"}}, nil
	}
	res, attempts, err := r.Run(context.Background(), []Step{step(1, 10), step(2, 20)})
	if err != nil || res == nil {
		t.Fatalf("err=%v", err)
	}
	// langkah 1: 2 percobaan (retry), lalu langkah 2 sukses
	if len(log) != 3 || log[0] != 1 || log[1] != 1 || log[2] != 2 {
		t.Errorf("urutan panggilan = %v, mau [1 1 2]", log)
	}
	if len(attempts) != 2 {
		t.Errorf("attempts = %d", len(attempts))
	}
}

func TestRunnerNoRetryOnBadRequest(t *testing.T) {
	r := &Runner{Breakers: NewBreakers(), MaxRetrySameProvider: 1}
	calls := 0
	r.Sender = func(ctx context.Context, s Step) (*AttemptResult, error) {
		calls++
		return nil, &translate.UpstreamError{StatusCode: 400, Message: "bad request"}
	}
	_, attempts, err := r.Run(context.Background(), []Step{step(1, 10), step(2, 20)})
	if err == nil {
		t.Fatal("harus gagal")
	}
	// 400 tidak di-retry ke credential lain, tapi tetap lanjut ke langkah combo berikutnya (FR-2.2 tidak memicu fallback utk 400!)
	// Per PRD: fallback TIDAK dipicu 400 → berarti 400 menghentikan seluruh rantai.
	// Karena 400 adalah kesalahan klien, mencoba provider lain takkan menolong.
	if calls != 1 {
		t.Errorf("400 tidak boleh lanjut langkah berikut: calls=%d attempts=%d", calls, len(attempts))
	}
}

func TestRunnerRetryCountPerStep(t *testing.T) {
	r := &Runner{Breakers: NewBreakers(), MaxRetrySameProvider: 1}
	calls := 0
	r.Sender = func(ctx context.Context, s Step) (*AttemptResult, error) {
		calls++
		return nil, &translate.UpstreamError{StatusCode: 500, Message: "boom"}
	}
	steps := []Step{step(1, 10), step(2, 20)}
	_, attempts, err := r.Run(context.Background(), steps)
	if err == nil {
		t.Fatal("harus gagal")
	}
	// per langkah 2 percobaan (1 retry), dua langkah → 4
	if calls != 4 {
		t.Errorf("calls = %d, mau 4", calls)
	}
	if len(attempts) != 4 {
		t.Errorf("attempts = %d", len(attempts))
	}
	// error terakhir diteruskan
	var ue *UpstreamError
	if !errors.As(err, &ue) || ue.StatusCode != 500 {
		t.Errorf("err = %v", err)
	}
}

func TestRunnerSkipsOpenProvider(t *testing.T) {
	r := &Runner{Breakers: NewBreakers(), MaxRetrySameProvider: 1}
	// buka breaker provider 10
	br := r.Breakers.For(10)
	for i := 0; i < 5; i++ {
		br.RecordFailure()
	}
	calls := 0
	r.Sender = func(ctx context.Context, s Step) (*AttemptResult, error) {
		calls++
		return &AttemptResult{Response: &translate.ChatResponse{Content: "ok"}}, nil
	}
	res, attempts, err := r.Run(context.Background(), []Step{step(1, 10), step(2, 20)})
	if err != nil || res == nil {
		t.Fatalf("err=%v", err)
	}
	if calls != 1 {
		t.Errorf("provider terbuka harus dilewati: calls=%d", calls)
	}
	if len(attempts) != 1 {
		t.Errorf("attempt skip = %d", len(attempts))
	}
}

func TestRunnerEmptySteps(t *testing.T) {
	r := &Runner{Breakers: NewBreakers()}
	_, _, err := r.Run(context.Background(), nil)
	if err != ErrNoStep {
		t.Errorf("err = %v", err)
	}
}

func TestRunnerContextCanceled(t *testing.T) {
	r := &Runner{Breakers: NewBreakers(), MaxRetrySameProvider: 1}
	ctx, cancel := context.WithCancel(context.Background())
	r.Sender = func(ctx context.Context, s Step) (*AttemptResult, error) {
		cancel()
		return nil, &translate.UpstreamError{StatusCode: 429, Message: "rl"}
	}
	_, _, err := r.Run(ctx, []Step{step(1, 10), step(2, 20)})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}
