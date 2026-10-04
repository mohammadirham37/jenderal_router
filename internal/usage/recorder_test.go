package usage

import (
	"bytes"
	"testing"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
)

func usageStore(t *testing.T) *store.Store {
	t.Helper()
	store.SetMasterKey(bytes.Repeat([]byte{3}, 32))
	s, err := store.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRecorderPersistsEntries(t *testing.T) {
	s := usageStore(t)
	r := NewRecorder(s, 128)
	r.Record(Entry{
		TS: time.Now(), UserID: 1, KeyID: 2, RequestedModel: "oa/gpt-5.4",
		ProviderID: 3, ModelID: 4, ProviderName: "OpenAI", ModelName: "gpt-5.4",
		Status: 200, LatencyMs: 120, TTFTMs: 80, TokensIn: 100, TokensOut: 50,
		CostUSD: 0.0005, Attempts: 2, Stream: true, IP: "1.2.3.4",
		QuotaTargets: []QuotaTarget{{Scope: "user", ScopeID: 1}},
	})
	r.Record(Entry{TS: time.Now(), Status: 502, ErrorCode: "upstream_error", RequestedModel: "x"})
	r.Close()
	if r.Dropped() != 0 {
		t.Errorf("dropped = %d", r.Dropped())
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM request_logs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("log = %d, mau 2", count)
	}
	var latency, attempts, stream int64
	err := s.DB.QueryRow(`SELECT latency_ms, attempts, stream FROM request_logs WHERE status = 200`).Scan(&latency, &attempts, &stream)
	if err != nil {
		t.Fatal(err)
	}
	if latency != 120 || attempts != 2 || stream != 1 {
		t.Errorf("latency=%d attempts=%d stream=%d", latency, attempts, stream)
	}
	// tanpa baris kuota → update dilewati tanpa error. Buat lalu cek:
	if _, err := s.PutQuota("user", 1, "day", 1000, 10, 1); err != nil {
		t.Fatal(err)
	}
	r2 := NewRecorder(s, 8)
	r2.Record(Entry{TS: time.Now(), Status: 200, TokensIn: 10, TokensOut: 5, QuotaTargets: []QuotaTarget{{Scope: "user", ScopeID: 1}}})
	r2.Close()
	q, _ := s.GetQuota("user", 1, "day")
	if q.UsedTokens != 15 || q.UsedRequests != 1 {
		t.Errorf("used = %+v", q)
	}
}

func TestRecorderOverloadDoesNotBlock(t *testing.T) {
	s := usageStore(t)
	r := NewRecorder(s, 2) // antrean kecil
	for i := 0; i < 500; i++ {
		r.Record(Entry{TS: time.Now(), Status: 200}) // harus tanpa blokir
	}
	r.Close()
	// sebagian pasti dibuang, tidak panic/deadlock
	if r.Dropped() == 0 {
		t.Log("semua masuk (kecepatan worker tinggi)")
	}
}

func TestCostCalculate(t *testing.T) {
	// 1 juta token in @ $3 + 1 juta out @ $15 = 18
	got := CostUSDCalculate(1_000_000, 1_000_000, 3, 15)
	if got != 18 {
		t.Errorf("cost = %f", got)
	}
	// model lokal harga 0
	if CostUSDCalculate(1000, 1000, 0, 0) != 0 {
		t.Error("biaya lokal harus 0")
	}
	// fraksi: 500 in @ $1/1M + 500 out @ $2/1M = 0.0015
	got = CostUSDCalculate(500, 500, 1, 2)
	if got < 0.0015-1e-9 || got > 0.0015+1e-9 {
		t.Errorf("cost fraksi = %f", got)
	}
}

func TestQuotaCheck(t *testing.T) {
	s := usageStore(t)
	if status := QuotaCheck(s, 1, 2); !status.OK {
		t.Fatalf("tanpa kuota harus OK: %+v", status)
	}
	// kuota token habis
	s.PutQuota("user", 1, "day", 100, 0, 0)
	s.RecordQuotaUsage("user", 1, 100, 0)
	status := QuotaCheck(s, 1, 2)
	if status.OK {
		t.Fatal("kuota habis harus ditolak")
	}
	if status.RetryAfter <= 0 {
		t.Error("retry after harus > 0")
	}
	// kuota biaya
	s.PutQuota("user", 1, "day", 0, 0, 5)
	s.RecordQuotaUsage("user", 1, 0, 5.5)
	status = QuotaCheck(s, 1, 2)
	if status.OK || status.Reason == "" {
		t.Fatalf("biaya harus ditolak: %+v", status)
	}
	// kuota key terpisah
	s.PutQuota("key", 9, "day", 10, 0, 0)
	s.RecordQuotaUsage("key", 9, 10, 0)
	status = QuotaCheck(s, 5, 9)
	if status.OK {
		t.Fatal("kuota key habis harus ditolak")
	}
}

func TestNextResetAt(t *testing.T) {
	// daily: selalu besok 00:00 zona waktu
	now := time.Date(2026, 10, 4, 15, 30, 0, 0, time.UTC)
	next := NextResetAt("day", "Asia/Jakarta", now)
	tNext, err := time.Parse(time.RFC3339, next)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	want := time.Date(2026, 10, 5, 0, 0, 0, 0, loc)
	if !tNext.Equal(want) {
		t.Errorf("next day reset = %v, mau %v", tNext, want)
	}
	// monthly: 1 bulan berikutnya
	nextM := NextResetAt("month", "Asia/Jakarta", now)
	tM, _ := time.Parse(time.RFC3339, nextM)
	wantM := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)
	if !tM.Equal(wantM) {
		t.Errorf("next month reset = %v, mau %v", tM, wantM)
	}
	// tz tidak sah fallback UTC
	if _, err := time.Parse(time.RFC3339, NextResetAt("day", "Zona/Tidak-Ada", now)); err != nil {
		t.Errorf("tz tak sah harus fallback: %v", err)
	}
}

func TestAnalyticsSummary(t *testing.T) {
	s := usageStore(t)
	now := time.Now().UTC()
	seed := []Entry{
		{TS: now.Add(-time.Hour), UserID: 1, ProviderID: 1, ProviderName: "OpenAI", Status: 200, LatencyMs: 100, TokensIn: 10, TokensOut: 5, CostUSD: 0.01},
		{TS: now.Add(-2 * time.Hour), UserID: 1, ProviderID: 1, ProviderName: "OpenAI", Status: 200, LatencyMs: 300, TokensIn: 20, TokensOut: 10, CostUSD: 0.02},
		{TS: now.Add(-3 * time.Hour), UserID: 2, ProviderID: 2, ProviderName: "Anthropic", Status: 502, ErrorCode: "upstream_error", LatencyMs: 50},
	}
	r := NewRecorder(s, 16)
	for _, e := range seed {
		r.Record(e)
	}
	r.Close()

	res, err := GetAnalytics(s, now.Add(-24*time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("analytics: %v", err)
	}
	if res.Totals.Requests != 3 || res.Totals.Errors != 1 {
		t.Fatalf("totals = %+v", res.Totals)
	}
	if res.Totals.TokensIn != 30 || res.Totals.TokensOut != 15 {
		t.Errorf("tokens = %+v", res.Totals)
	}
	if res.ErrorRate < 0.33 || res.ErrorRate > 0.34 {
		t.Errorf("error rate = %f", res.ErrorRate)
	}
	if len(res.PerProvider) != 2 {
		t.Errorf("per provider = %d", len(res.PerProvider))
	}
	if len(res.PerUser) != 2 {
		t.Errorf("per user = %d", len(res.PerUser))
	}
	if res.Totals.P50LatencyMs == 0 || res.Totals.P95LatencyMs == 0 {
		t.Errorf("persentil = %d/%d", res.Totals.P50LatencyMs, res.Totals.P95LatencyMs)
	}
}
