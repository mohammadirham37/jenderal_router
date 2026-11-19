package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAdminDashboardAndMeActivity(t *testing.T) {
	app, st, _ := appTestEnv(t)
	seedMockProvider(t, st, "Mock", "mk", "http://127.0.0.1:1")
	var provID int64
	st.DB.QueryRow(`SELECT id FROM providers WHERE prefix = 'mk'`).Scan(&provID)
	dev, _ := st.GetUserByEmail("dev@test")
	var keyID int64
	st.DB.QueryRow(`SELECT id FROM api_keys WHERE user_id = ?`, dev.ID).Scan(&keyID)
	ins := func(ago time.Duration, status int, viaKey bool, tokens int) {
		var key any
		if viaKey {
			key = keyID
		}
		_, err := st.DB.Exec(`INSERT INTO request_logs (ts, user_id, key_id, requested_model, provider_id, provider_name, status, error_code, latency_ms, tokens_in, tokens_out, cost_usd)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, time.Now().Add(-ago).UTC().Format(time.RFC3339Nano), dev.ID, key,
			"mk/mock-model", provID, "Mock", status, map[bool]string{true: "upstream_error"}[status >= 400], 120, tokens, tokens, 0.01)
		if err != nil {
			t.Fatal(err)
		}
	}
	// periode berjalan: 6 request, 4 gagal dalam 1 jam terakhir; periode lalu: 2 request
	for i := 0; i < 2; i++ {
		ins(10*time.Minute, 200, i == 0, 10)
	}
	for i := 0; i < 4; i++ {
		ins(5*time.Minute, 502, true, 0)
	}
	ins(8*24*time.Hour, 200, true, 5)
	ins(9*24*time.Hour, 200, true, 5)
	// kuota hampir habis
	st.PutQuota("user", dev.ID, "day", 100, 0, 0)
	st.RecordQuotaUsage("user", dev.ID, 95, 0)

	cookie, _ := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	w := dashReq(app, "GET", "/api/admin/dashboard?range=7d", nil, cookie, "")
	if w.Code != 200 {
		t.Fatalf("dashboard = %d %s", w.Code, w.Body.String())
	}
	var d struct {
		Current, Previous struct{ Requests, Errors int64 }
		Series            []struct{ T string }
		TopModels         []struct {
			Model    string
			Requests int64
		} `json:"top_models"`
		Providers    []struct{ Name, Status string }
		RecentErrors []struct {
			Via       string
			ErrorCode string `json:"error_code"`
		} `json:"recent_errors"`
		Alerts []struct{ Level, Kind, Text string }
	}
	json.Unmarshal(w.Body.Bytes(), &d)
	if d.Current.Requests != 6 || d.Current.Errors != 4 || d.Previous.Requests != 2 {
		t.Errorf("totals cur=%+v prev=%+v", d.Current, d.Previous)
	}
	if len(d.Series) != 8 && len(d.Series) != 7 { // 7 hari ke belakang + hari ini (zona lokal)
		t.Errorf("series len = %d", len(d.Series))
	}
	if len(d.TopModels) != 1 || d.TopModels[0].Requests != 6 {
		t.Errorf("top models = %+v", d.TopModels)
	}
	if len(d.Providers) != 1 || d.Providers[0].Status != "degraded" {
		t.Errorf("providers = %+v", d.Providers)
	}
	if len(d.RecentErrors) != 4 || d.RecentErrors[0].Via != "api" || d.RecentErrors[0].ErrorCode != "upstream_error" {
		t.Errorf("recent errors = %+v", d.RecentErrors)
	}
	kinds := map[string]bool{}
	for _, a := range d.Alerts {
		kinds[a.Kind] = true
	}
	if !kinds["provider"] || !kinds["errors"] || !kinds["quota"] {
		t.Errorf("alerts = %+v", d.Alerts)
	}

	w = dashReq(app, "GET", "/api/admin/dashboard?range=24h", nil, cookie, "")
	var h struct {
		Hourly bool
		Series []struct{ T string }
	}
	json.Unmarshal(w.Body.Bytes(), &h)
	if !h.Hourly || len(h.Series) < 24 || len(h.Series) > 26 || !strings.Contains(h.Series[0].T, ":00") {
		t.Errorf("hourly series = %v len %d", h.Hourly, len(h.Series))
	}

	// member: hanya miliknya sendiri, chat vs API dipisah
	devCookie, _ := loginAs(app, t, "dev@test", "Sandi-Kuat-123")
	w = dashReq(app, "GET", "/api/me/activity?days=7", nil, devCookie, "")
	var m struct {
		Series []struct{ T string }
		Total  struct{ Requests, Chat, API int64 }
	}
	json.Unmarshal(w.Body.Bytes(), &m)
	if len(m.Series) != 7 || m.Total.Requests != 6 || m.Total.Chat != 1 || m.Total.API != 5 {
		t.Errorf("me activity = %d series, total %+v", len(m.Series), m.Total)
	}
	w = dashReq(app, "GET", "/api/admin/nav-status", nil, cookie, "")
	if !strings.Contains(w.Body.String(), `"providers_degraded":1`) || !strings.Contains(w.Body.String(), `"errors_hour":4`) ||
		!strings.Contains(w.Body.String(), `"quota_alerts":1`) {
		t.Errorf("nav-status = %s", w.Body.String())
	}
	if w := dashReq(app, "GET", "/api/admin/nav-status", nil, devCookie, ""); w.Code != 403 {
		t.Errorf("member nav-status = %d", w.Code)
	}
	if w := dashReq(app, "GET", "/api/admin/dashboard", nil, devCookie, ""); w.Code != 403 {
		t.Errorf("member access admin dashboard = %d", w.Code)
	}
}

func TestListUsersSummary(t *testing.T) {
	app, st, _ := appTestEnv(t)
	dev, _ := st.GetUserByEmail("dev@test")
	st.PutQuota("user", dev.ID, "day", 100, 0, 0)
	st.RecordQuotaUsage("user", dev.ID, 40, 0)
	st.DB.Exec(`INSERT INTO request_logs (ts, user_id, requested_model, status) VALUES (?,?,?,?)`, "2026-11-01T10:00:00Z", dev.ID, "m", 200)
	st.SetUserLogContent(dev.ID, true)
	st.CreateUser("admin2@test", "Kuat-2026-Pass", "admin")

	cookie, _ := loginAs(app, t, "admin2@test", "Kuat-2026-Pass")
	w := dashReq(app, "GET", "/api/admin/users", nil, cookie, "")
	var res struct {
		Users []struct {
			User      struct{ Email string }
			CanManage bool `json:"can_manage"`
			Summary   struct {
				KeysActive  int     `json:"keys_active"`
				LastActive  string  `json:"last_active"`
				QuotaPct    float64 `json:"quota_pct"`
				QuotaLabel  string  `json:"quota_label"`
				QuotaSource string  `json:"quota_source"`
				LogContent  bool    `json:"log_content"`
			}
		}
	}
	json.Unmarshal(w.Body.Bytes(), &res)
	got := map[string]int{}
	for i, u := range res.Users {
		got[u.User.Email] = i
	}
	d := res.Users[got["dev@test"]]
	if d.Summary.KeysActive != 1 || d.Summary.QuotaPct != 40 || d.Summary.QuotaLabel != "token harian" ||
		d.Summary.QuotaSource != "custom" || d.Summary.LastActive == "" || !d.Summary.LogContent || !d.CanManage {
		t.Errorf("dev summary = %+v can=%v", d.Summary, d.CanManage)
	}
	if res.Users[got["admin@test"]].CanManage {
		t.Error("admin must not manage super_admin")
	}
}
