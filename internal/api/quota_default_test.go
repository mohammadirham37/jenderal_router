package api

import (
	"strings"
	"testing"
	"time"

	"github.com/jenderal/jenderalrouter/internal/usage"
)

func TestDefaultQuotaAPIAndMeUsage(t *testing.T) {
	app, st, _ := appTestEnv(t)
	cookie, csrf := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	w := dashReq(app, "PUT", "/api/admin/quota-default", map[string]any{
		"day": map[string]any{"token_limit": 2000, "request_limit": 20}, "month": map[string]any{"cost_limit_usd": 5},
	}, cookie, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"users_default":1`) {
		t.Fatalf("put default = %d %s", w.Code, w.Body.String())
	}
	if w := dashReq(app, "PUT", "/api/admin/quota-default", map[string]any{"week": map[string]any{"token_limit": 1}}, cookie, csrf); w.Code != 400 {
		t.Errorf("unknown period = %d", w.Code)
	}

	// user lama (dev) langsung kena + terjadwal reset; dashboard-nya menampilkan kuota
	dev, _ := st.GetUserByEmail("dev@test")
	q, err := st.GetQuota("user", dev.ID, "day")
	if err != nil || q.ResetAt == "" || q.Source != "default" {
		t.Fatalf("dev day quota = %+v, %v", q, err)
	}
	devCookie, _ := loginAs(app, t, "dev@test", "Sandi-Kuat-123")
	w = dashReq(app, "GET", "/api/me/usage", nil, devCookie, "")
	if !strings.Contains(w.Body.String(), `"source":"default"`) || !strings.Contains(w.Body.String(), `"token_limit":2000`) ||
		!strings.Contains(w.Body.String(), `"cost_limit_usd":5`) {
		t.Errorf("me/usage = %s", w.Body.String())
	}

	// user baru lewat endpoint admin langsung punya kuota default
	w = dashReq(app, "POST", "/api/admin/users", map[string]any{"email": "baru@test", "password": "Sandi-Kuat-123", "role": "member"}, cookie, csrf)
	if w.Code != 201 && w.Code != 200 {
		t.Fatalf("create user = %d %s", w.Code, w.Body.String())
	}
	baru, _ := st.GetUserByEmail("baru@test")
	w = dashReq(app, "GET", "/api/admin/users/"+fmtInt(baru.ID)+"/quota", nil, cookie, "")
	if !strings.Contains(w.Body.String(), `"source":"default"`) {
		t.Errorf("new user quota = %s", w.Body.String())
	}

	// kuota khusus lalu kembali ke default
	dashReq(app, "PUT", "/api/admin/users/"+fmtInt(baru.ID)+"/quota", map[string]any{"period": "day", "token_limit": 7}, cookie, csrf)
	if q, _ := st.GetQuota("user", baru.ID, "day"); q.Source != "custom" || q.TokenLimit != 7 {
		t.Errorf("custom = %+v", q)
	}
	if w := dashReq(app, "DELETE", "/api/admin/users/"+fmtInt(baru.ID)+"/quota?period=day", nil, cookie, csrf); w.Code != 200 {
		t.Fatalf("use default = %d %s", w.Code, w.Body.String())
	}
	if q, _ := st.GetQuota("user", baru.ID, "day"); q.Source != "default" || q.TokenLimit != 2000 || q.ResetAt == "" {
		t.Errorf("back to default = %+v", q)
	}
	if w := dashReq(app, "GET", "/api/admin/audit", nil, cookie, ""); !strings.Contains(w.Body.String(), "quota.default") {
		t.Error("default quota change not audited")
	}
}

// Kuota lama tanpa jadwal reset (bug sebelumnya) dijadwalkan & dinolkan.
func TestScheduleMissingResets(t *testing.T) {
	_, st, _ := appTestEnv(t)
	dev, _ := st.GetUserByEmail("dev@test")
	q, _ := st.PutQuota("user", dev.ID, "day", 100, 0, 0)
	st.RecordQuotaUsage("user", dev.ID, 150, 0) // "habis" selamanya sebelum perbaikan
	if usage.QuotaCheck(st, dev.ID, 0).OK {
		t.Fatal("precondition: quota should be exhausted")
	}
	now := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	usage.ScheduleMissingResets(st, "Asia/Jakarta", now)
	got, _ := st.GetQuota("user", dev.ID, "day")
	if got.ResetAt != "2026-10-31T17:00:00Z" || got.UsedTokens != 0 {
		t.Fatalf("after schedule = %+v (id %d)", got, q.ID)
	}
	if !usage.QuotaCheck(st, dev.ID, 0).OK {
		t.Error("quota must be usable again after scheduling")
	}
}
