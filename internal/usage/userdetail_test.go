package usage

import (
	"testing"
	"time"
)

func TestGetUserDetailScopedToUser(t *testing.T) {
	s := usageStore(t)
	r := NewRecorder(s, 64)
	now := time.Now()
	// user 1: dua model, dua key, satu error
	r.Record(Entry{TS: now, UserID: 1, KeyID: 10, RequestedModel: "oa/gpt", ProviderName: "OpenAI", ModelName: "gpt",
		Status: 200, LatencyMs: 100, TokensIn: 10, TokensOut: 5, CostUSD: 0.01})
	r.Record(Entry{TS: now, UserID: 1, KeyID: 10, RequestedModel: "oa/gpt", ProviderName: "OpenAI", ModelName: "gpt",
		Status: 502, LatencyMs: 300, ErrorCode: "upstream_error"})
	r.Record(Entry{TS: now, UserID: 1, KeyID: 11, RequestedModel: "local/qwen", ProviderName: "Local", ModelName: "qwen",
		Status: 200, LatencyMs: 200, TokensIn: 20, TokensOut: 20})
	// user 2: tidak boleh ikut terhitung
	r.Record(Entry{TS: now, UserID: 2, KeyID: 12, RequestedModel: "oa/gpt", Status: 200, TokensIn: 999, CostUSD: 9})
	// di luar rentang
	r.Record(Entry{TS: now.AddDate(0, 0, -30), UserID: 1, KeyID: 10, RequestedModel: "oa/gpt", Status: 200, TokensIn: 777})
	r.Close()

	d, err := GetUserDetail(s, 1, now.AddDate(0, 0, -7), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if d.Totals.Requests != 3 || d.Totals.Errors != 1 {
		t.Fatalf("totals req=%d err=%d, mau 3/1", d.Totals.Requests, d.Totals.Errors)
	}
	if d.Totals.TokensIn != 30 || d.Totals.TokensOut != 25 {
		t.Fatalf("tokens in=%d out=%d", d.Totals.TokensIn, d.Totals.TokensOut)
	}
	if d.Totals.P95LatencyMs != 200 || d.Totals.P50LatencyMs != 200 {
		t.Errorf("p50=%d p95=%d", d.Totals.P50LatencyMs, d.Totals.P95LatencyMs)
	}
	if len(d.PerDay) != 1 {
		t.Errorf("per_day = %d baris", len(d.PerDay))
	}
	if len(d.PerModel) != 2 || d.PerModel[0].Model != "oa/gpt" || d.PerModel[0].Requests != 2 || d.PerModel[0].Errors != 1 {
		t.Errorf("per_model = %+v", d.PerModel)
	}
	if len(d.PerKey) != 2 || d.PerKey[0].KeyID != 10 || d.PerKey[0].Requests != 2 {
		t.Errorf("per_key = %+v", d.PerKey)
	}
	if d.ErrorRate < 0.33 || d.ErrorRate > 0.34 {
		t.Errorf("error_rate = %v", d.ErrorRate)
	}
}

// Hari analitik mengikuti JR_TZ: 23:30 UTC = 06:30 WIB hari berikutnya.
func TestDayGroupingFollowsTimeZone(t *testing.T) {
	s := usageStore(t)
	jkt, _ := time.LoadLocation("Asia/Jakarta")
	SetLocation(jkt)
	t.Cleanup(func() { SetLocation(time.UTC) })
	r := NewRecorder(s, 8)
	ts := time.Date(2026, 10, 28, 23, 30, 0, 0, time.UTC)
	r.Record(Entry{TS: ts, UserID: 1, RequestedModel: "m", Status: 200})
	r.Close()
	d, err := GetUserDetail(s, 1, ts.Add(-time.Hour), ts.Add(time.Hour))
	if err != nil || len(d.PerDay) != 1 || d.PerDay[0].Day != "2026-10-29" {
		t.Fatalf("per_day = %+v, %v; mau 2026-10-29 (WIB)", d.PerDay, err)
	}
	if got := StartOfToday(ts); !got.Equal(time.Date(2026, 10, 29, 0, 0, 0, 0, jkt)) {
		t.Fatalf("StartOfToday = %v", got)
	}
	u, err := UserTotals(s, 1, ts.Add(-time.Hour), ts.Add(time.Hour))
	if err != nil || u.Requests != 1 {
		t.Fatalf("UserTotals = %+v, %v", u, err)
	}
	if n, err := PurgeLogs(s, ts.Add(time.Minute)); err != nil || n != 1 {
		t.Fatalf("PurgeLogs = %d, %v", n, err)
	}
}
