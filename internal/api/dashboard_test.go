package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jenderal/jenderalrouter/internal/store"
)

// ---- helper dashboard ----

func dashReq(app *App, method, path string, body any, cookie, csrf string) *httptest.ResponseRecorder {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, path, rd)
	r.RemoteAddr = "198.51.100.1:7777"
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	if csrf != "" {
		r.Header.Set("X-CSRF-Token", csrf)
	}
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, r)
	return w
}

func loginAs(app *App, t *testing.T, email, password string) (cookie, csrf string) {
	t.Helper()
	w := dashReq(app, "POST", "/api/login", map[string]string{"email": email, "password": password}, "", "")
	if w.Code != 200 {
		t.Fatalf("login %s: status=%d body=%s", email, w.Code, w.Body.String())
	}
	var resp struct {
		CSRF string `json:"csrf"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			return c.Value, resp.CSRF
		}
	}
	t.Fatal("cookie sesi tidak diset")
	return "", ""
}

func TestSetupWizardFlow(t *testing.T) {
	app, _, _ := appTestEnv(t)
	// status awal: appTestEnv sudah membuat super admin → needs_setup false
	w := dashReq(app, "GET", "/api/setup/status", nil, "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"needs_setup":false`) {
		t.Fatalf("status = %s", w.Body.String())
	}
	// setup kedua ditolak
	w = dashReq(app, "POST", "/api/setup", map[string]string{"email": "x@y.z", "password": "Sandi-Kuat-123"}, "", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("setup kedua harus ditolak: %d", w.Code)
	}
}

func TestSetupPasswordWeakRejected(t *testing.T) {
	app, st, _ := appTestEnv(t)
	// buat store baru tanpa super admin: hapus semua user
	st.DB.Exec(`DELETE FROM users`)
	w := dashReq(app, "GET", "/api/setup/status", nil, "", "")
	if !strings.Contains(w.Body.String(), `"needs_setup":true`) {
		t.Fatalf("harus needs_setup: %s", w.Body.String())
	}
	// password lemah ditolak (NFR-02)
	w = dashReq(app, "POST", "/api/setup", map[string]string{"email": "root@x", "password": "lemah"}, "", "")
	if w.Code != 400 {
		t.Fatalf("password lemah harus ditolak: %d %s", w.Code, w.Body.String())
	}
	// password kuat diterima + cookie sesi
	w = dashReq(app, "POST", "/api/setup", map[string]string{"email": "root@x", "password": "Kuat-2026-Pass"}, "", "")
	if w.Code != 200 {
		t.Fatalf("setup gagal: %d %s", w.Code, w.Body.String())
	}
	if len(w.Result().Cookies()) == 0 {
		t.Error("cookie sesi harus diset")
	}
}

func TestLoginLogoutMe(t *testing.T) {
	app, _, _ := appTestEnv(t)
	// password salah
	w := dashReq(app, "POST", "/api/login", map[string]string{"email": "admin@test", "password": "Salah-12345678"}, "", "")
	if w.Code != 401 {
		t.Fatalf("password salah harus 401: %d", w.Code)
	}
	cookie, csrf := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	w = dashReq(app, "GET", "/api/me", nil, cookie, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "admin@test") {
		t.Fatalf("/api/me = %s", w.Body.String())
	}
	// logout tanpa CSRF → 403
	w = dashReq(app, "POST", "/api/logout", nil, cookie, "")
	if w.Code != 403 {
		t.Fatalf("tanpa CSRF harus 403: %d", w.Code)
	}
	w = dashReq(app, "POST", "/api/logout", nil, cookie, csrf)
	if w.Code != 200 {
		t.Fatalf("logout: %d", w.Code)
	}
	// setelah logout sesi tidak berlaku
	w = dashReq(app, "GET", "/api/me", nil, cookie, "")
	if w.Code != 401 {
		t.Fatalf("sesi harus mati: %d", w.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	app, st, _ := appTestEnv(t)
	st.DB.Exec(`DELETE FROM users`) // bersihkan agar login selalu gagal konsisten
	for i := 0; i < 5; i++ {
		w := dashReq(app, "POST", "/api/login", map[string]string{"email": "brute@x", "password": "Salah-123456789"}, "", "")
		if w.Code != 401 {
			t.Fatalf("percobaan %d: %d", i, w.Code)
		}
	}
	// percobaan ke-6 → 429 (NFR-05)
	w := dashReq(app, "POST", "/api/login", map[string]string{"email": "brute@x", "password": "Salah-123456789"}, "", "")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("harus 429: %d", w.Code)
	}
}

func TestAdminAPIRequiresAuth(t *testing.T) {
	app, _, _ := appTestEnv(t)
	w := dashReq(app, "GET", "/api/admin/providers", nil, "", "")
	if w.Code != 401 {
		t.Fatalf("tanpa sesi harus 401: %d", w.Code)
	}
	cookie, csrf := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	w = dashReq(app, "GET", "/api/admin/providers", nil, cookie, "")
	if w.Code != 200 {
		t.Fatalf("dengan sesi harus 200: %d %s", w.Code, w.Body.String())
	}
	// mutasi tanpa CSRF → 403
	w = dashReq(app, "POST", "/api/admin/combos", map[string]any{"name": "x", "model_ids": []int64{1}}, cookie, "")
	if w.Code != 403 {
		t.Fatalf("mutasi tanpa CSRF harus 403: %d", w.Code)
	}
	_ = csrf
}

func TestProviderSeedAndList(t *testing.T) {
	app, st, _ := appTestEnv(t)
	cookie, csrf := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	_ = st
	w := dashReq(app, "POST", "/api/admin/providers/seed", map[string]any{"prefix": "an", "api_key": "sk-an-123"}, cookie, csrf)
	if w.Code != 201 {
		t.Fatalf("seed: %d %s", w.Code, w.Body.String())
	}
	// seed ulang → idempoten (200)
	w = dashReq(app, "POST", "/api/admin/providers/seed", map[string]any{"prefix": "an", "api_key": "sk-an-456"}, cookie, csrf)
	if w.Code != 200 {
		t.Fatalf("seed ulang: %d %s", w.Code, w.Body.String())
	}
	w = dashReq(app, "GET", "/api/admin/models", nil, cookie, "")
	if !strings.Contains(w.Body.String(), "an/claude-sonnet-4-6") {
		t.Errorf("model template harus terdaftar: %s", w.Body.String())
	}
	// template list
	w = dashReq(app, "GET", "/api/admin/templates", nil, cookie, "")
	if !strings.Contains(w.Body.String(), "LlamaStash") {
		t.Errorf("template harus memuat LlamaStash: %s", w.Body.String())
	}
}

func TestUserKeyQuotaManagement(t *testing.T) {
	app, st, _ := appTestEnv(t)
	cookie, csrf := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	_ = st

	// buat user
	w := dashReq(app, "POST", "/api/admin/users", map[string]any{
		"email": "magang@test", "password": "Kuat-2026-Pass", "role": "member",
	}, cookie, csrf)
	if w.Code != 201 {
		t.Fatalf("buat user: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		User struct {
			ID int64 `json:"id"`
		} `json:"user"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	// buat key → plaintext tampil sekali
	w = dashReq(app, "POST", "/api/admin/users/"+fmtInt(resp.User.ID)+"/keys", map[string]any{
		"name": "kunci magang", "allowed_models": "*", "rpm": 60,
	}, cookie, csrf)
	if w.Code != 201 {
		t.Fatalf("buat key: %d %s", w.Code, w.Body.String())
	}
	var keyResp struct {
		Plaintext string `json:"plaintext"`
	}
	json.Unmarshal(w.Body.Bytes(), &keyResp)
	if !strings.HasPrefix(keyResp.Plaintext, "jr-") {
		t.Fatalf("plaintext key = %q", keyResp.Plaintext)
	}
	// list key TIDAK memuat plaintext
	w = dashReq(app, "GET", "/api/admin/users/"+fmtInt(resp.User.ID)+"/keys", nil, cookie, "")
	if strings.Contains(w.Body.String(), keyResp.Plaintext) {
		t.Fatal("list key tidak boleh memuat plaintext")
	}

	// put kuota user
	w = dashReq(app, "PUT", "/api/admin/users/"+fmtInt(resp.User.ID)+"/quota", map[string]any{
		"period": "day", "token_limit": 200000,
	}, cookie, csrf)
	if w.Code != 200 {
		t.Fatalf("kuota: %d %s", w.Code, w.Body.String())
	}

	// hapus user
	w = dashReq(app, "DELETE", "/api/admin/users/"+fmtInt(resp.User.ID), nil, cookie, csrf)
	if w.Code != 200 {
		t.Fatalf("hapus user: %d", w.Code)
	}
}

func TestRBACMemberCannotManage(t *testing.T) {
	app, st, _ := appTestEnv(t)
	// buat admin (bukan super) + member
	_, _ = st.CreateUser("admin2@test", "Kuat-2026-Pass", store.RoleAdmin)
	_, _ = st.CreateUser("budi@test", "Kuat-2026-Pass", store.RoleMember)

	cookieAdmin, csrfAdmin := loginAs(app, t, "admin2@test", "Kuat-2026-Pass")
	// admin boleh kelola member
	w := dashReq(app, "POST", "/api/admin/users", map[string]any{
		"email": "cici@test", "password": "Kuat-2026-Pass", "role": "member",
	}, cookieAdmin, csrfAdmin)
	if w.Code != 201 {
		t.Fatalf("admin buat member: %d %s", w.Code, w.Body.String())
	}
	// admin tidak boleh mengangkat admin
	w = dashReq(app, "POST", "/api/admin/users", map[string]any{
		"email": "admin3@test", "password": "Kuat-2026-Pass", "role": "admin",
	}, cookieAdmin, csrfAdmin)
	if w.Code != http.StatusForbidden {
		t.Fatalf("admin buat admin harus 403: %d", w.Code)
	}

	// member tidak boleh akses admin API
	cookieMember, _ := loginAs(app, t, "budi@test", "Kuat-2026-Pass")
	w = dashReq(app, "GET", "/api/admin/users", nil, cookieMember, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("member akses users harus 403: %d", w.Code)
	}
}

func TestLogsAndAnalyticsEndpoints(t *testing.T) {
	app, st, key := appTestEnv(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
	}))
	defer mock.Close()
	seedMockProvider(t, st, "Mock", "mk", mock.URL)
	doJSON(t, app.Handler(), "POST", "/v1/chat/completions", key, inferBody("mk/mock-model", false))
	app.rec.Close()

	cookie, _ := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	w := dashReq(app, "GET", "/api/admin/logs", nil, cookie, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "mk/mock-model") {
		t.Fatalf("logs = %s", w.Body.String())
	}
	// export CSV
	w = dashReq(app, "GET", "/api/admin/logs/export.csv", nil, cookie, "")
	if !strings.HasPrefix(w.Body.String(), "ts,user_id") {
		t.Fatalf("csv header = %s", w.Body.String())
	}
	// analytics
	w = dashReq(app, "GET", "/api/admin/analytics/summary?days=1", nil, cookie, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"totals"`) {
		t.Fatalf("analytics = %s", w.Body.String())
	}
}

func TestAuditRecorded(t *testing.T) {
	app, _, _ := appTestEnv(t)
	cookie, csrf := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	dashReq(app, "POST", "/api/admin/providers/seed", map[string]any{"prefix": "gm", "api_key": "g-key"}, cookie, csrf)
	w := dashReq(app, "GET", "/api/admin/audit", nil, cookie, "")
	if !strings.Contains(w.Body.String(), "provider.seed") {
		t.Fatalf("audit = %s", w.Body.String())
	}
}

func TestConversationsCRUD(t *testing.T) {
	app, _, _ := appTestEnv(t)
	cookie, csrf := loginAs(app, t, "admin@test", "Sandi-Kuat-123")

	w := dashReq(app, "POST", "/api/me/conversations", map[string]any{"title": "Uji", "model": "mk/m"}, cookie, csrf)
	if w.Code != 201 {
		t.Fatalf("create conv: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Conversation struct {
			ID int64 `json:"id"`
		} `json:"conversation"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	app.SaveConversationMessage(resp.Conversation.ID, "user", "halo", "mk/m", "Mock")
	app.SaveConversationMessage(resp.Conversation.ID, "assistant", "hai!", "mk/m", "Mock")

	w = dashReq(app, "GET", "/api/me/conversations/"+fmtInt(resp.Conversation.ID), nil, cookie, "")
	if !strings.Contains(w.Body.String(), "hai!") {
		t.Fatalf("messages = %s", w.Body.String())
	}
	w = dashReq(app, "DELETE", "/api/me/conversations/"+fmtInt(resp.Conversation.ID), nil, cookie, csrf)
	if w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
}
