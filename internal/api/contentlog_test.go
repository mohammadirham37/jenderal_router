package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jenderal/jenderalrouter/internal/crypto"
	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/translate"
)

func TestMaskSecrets(t *testing.T) {
	cases := map[string]string{
		"pakai sk-proj-abcdefghijklmnopqrstuv123 ya":        "pakai sk-p…[disamarkan] ya",
		"key jr-0123456789abcdefghijklmnopqrstuv0123456789": "key jr-0…[disamarkan]",
		"Authorization: Bearer abcdefghijklmnop.qrstuvwxyz": "Authorization: Bearer [disamarkan]",
		`{"password": "rahasia123"}`:                        `{"password": "[disamarkan]"}`,
		"kata sandi=Bismillah12":                            "kata sandi=[disamarkan]",
		"kartu 4111 1111 1111 1111 berlaku":                 "kartu [kartu ****1111] berlaku",
		"nomor 1234567890123 bukan kartu":                   "nomor 1234567890123 bukan kartu", // gagal Luhn
		"teks biasa tanpa rahasia":                          "teks biasa tanpa rahasia",
	}
	for in, want := range cases {
		if got := maskSecrets(in); got != want {
			t.Errorf("maskSecrets(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewContentLogLimits(t *testing.T) {
	big := strings.Repeat("a", maxLoggedMessage*2)
	req := &translate.ChatRequest{Model: "m", Messages: []translate.Message{
		{Role: "system", Content: []translate.Part{{Type: translate.PartText, Text: "sistem"}}},
	}}
	for i := 0; i < 12; i++ {
		req.Messages = append(req.Messages, translate.Message{Role: "user", Content: []translate.Part{
			{Type: translate.PartText, Text: fmt.Sprintf("pesan-%d ", i) + big},
			{Type: translate.PartImage, ImageURL: "data:image/png;base64,AAAA"},
		}})
	}
	c := newContentLog(req)
	if !c.Truncated {
		t.Error("expected truncated flag")
	}
	msgs := c.Request.Messages
	if msgs[0].Role != "system" || msgs[1].Role != "info" || !strings.Contains(msgs[1].Content, "pesan sebelumnya tidak disimpan") {
		t.Fatalf("head = %+v / %+v", msgs[0].Role, msgs[1])
	}
	last := msgs[len(msgs)-1]
	if !strings.HasPrefix(last.Content, "pesan-11 ") || !strings.Contains(last.Content, "[gambar]") || strings.Contains(last.Content, "base64") {
		t.Errorf("last message = %.80s…", last.Content)
	}
	if b := c.render(); len(b) > maxLoggedRequest+maxLoggedResponse+4096 {
		t.Errorf("rendered size %d too large", len(b))
	}
	var nilLog *contentLog
	if nilLog.render() != "" {
		t.Error("nil log must render empty")
	}
}

// contentEnv app + mock provider (stream & non-stream) + key user dev.
func contentEnv(t *testing.T) (*App, *store.Store, http.Handler, string, *store.APIKey) {
	t.Helper()
	app, st, _ := appTestEnv(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseText("jawaban stream"), sseToolCall("c1", "cari", `{"q":"x"}`), sseFinish("stop"))
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"jawaban biasa"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
	}))
	t.Cleanup(mock.Close)
	seedMockProvider(t, st, "Mock", "mk", mock.URL)
	dev, _ := st.GetUserByEmail("dev@test")
	plain, hash, _ := crypto.NewAPIKey()
	k, err := st.CreateAPIKey(dev.ID, "uji", plain, hash, "", "*", "", 0, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	return app, st, app.Handler(), plain, k
}

func chatBody(stream bool, text string) []byte {
	b, _ := json.Marshal(map[string]any{"model": "mk/mock-model", "stream": stream,
		"messages": []map[string]any{{"role": "user", "content": text}}})
	return b
}

// lastLogContent menunggu recorder menulis n baris lalu mengembalikan isi baris terakhir.
func lastLogContent(t *testing.T, st *store.Store, n int) (int64, string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var count int
		st.DB.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE key_id IS NOT NULL`).Scan(&count)
		if count >= n {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("log rows = %d, want %d", count, n)
		}
		time.Sleep(20 * time.Millisecond)
	}
	var id int64
	var c string
	st.DB.QueryRow(`SELECT id, content_debug FROM request_logs WHERE key_id IS NOT NULL ORDER BY id DESC LIMIT 1`).Scan(&id, &c)
	return id, c
}

func TestContentLoggingSwitches(t *testing.T) {
	app, st, h, plain, k := contentEnv(t)

	// semua saklar mati → isi tidak dicatat
	if w := doJSON(t, h, "POST", "/v1/chat/completions", plain, chatBody(false, "rahasia perusahaan")); w.Code != 200 {
		t.Fatalf("infer = %d %s", w.Code, w.Body.String())
	}
	if _, c := lastLogContent(t, st, 1); c != "" {
		t.Fatalf("content must be empty when logging off: %s", c)
	}

	// saklar per key (lewat endpoint admin, cache key ikut diperbarui)
	cookie, csrf := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	if w := dashReq(app, "PUT", "/api/admin/keys/"+fmtInt(k.ID)+"/log-content", map[string]any{"enabled": true}, cookie, csrf); w.Code != 200 {
		t.Fatalf("toggle key = %d %s", w.Code, w.Body.String())
	}
	doJSON(t, h, "POST", "/v1/chat/completions", plain, chatBody(false, "token=abcdef123456 tolong ringkas"))
	_, c := lastLogContent(t, st, 2)
	var got contentLog
	if err := json.Unmarshal([]byte(c), &got); err != nil {
		t.Fatalf("content json: %v (%s)", err, c)
	}
	if got.Request.Messages[0].Content != "token=[disamarkan] tolong ringkas" || got.Response.Content != "jawaban biasa" {
		t.Errorf("logged = %+v", got)
	}

	// stream: teks + tool call jawaban ikut tercatat
	if w := doJSON(t, h, "POST", "/v1/chat/completions", plain, chatBody(true, "halo stream")); w.Code != 200 {
		t.Fatalf("stream = %d", w.Code)
	}
	_, c = lastLogContent(t, st, 3)
	if !strings.Contains(c, `"content":"jawaban stream"`) || !strings.Contains(c, `"name":"cari"`) || !strings.Contains(c, `"stream":true`) {
		t.Errorf("stream log = %s", c)
	}

	// matikan key, nyalakan per user
	dashReq(app, "PUT", "/api/admin/keys/"+fmtInt(k.ID)+"/log-content", map[string]any{"enabled": false}, cookie, csrf)
	dev, _ := st.GetUserByEmail("dev@test")
	if w := dashReq(app, "PUT", "/api/admin/users/"+fmtInt(dev.ID)+"/log-content", map[string]any{"enabled": true}, cookie, csrf); w.Code != 200 {
		t.Fatalf("toggle user = %d %s", w.Code, w.Body.String())
	}
	doJSON(t, h, "POST", "/v1/chat/completions", plain, chatBody(false, "via saklar user"))
	if _, c = lastLogContent(t, st, 4); !strings.Contains(c, "via saklar user") {
		t.Errorf("user switch not applied: %q", c)
	}

	// matikan user, nyalakan global (settings)
	dashReq(app, "PUT", "/api/admin/users/"+fmtInt(dev.ID)+"/log-content", map[string]any{"enabled": false}, cookie, csrf)
	doJSON(t, h, "POST", "/v1/chat/completions", plain, chatBody(false, "semua mati"))
	if _, c = lastLogContent(t, st, 5); c != "" {
		t.Errorf("all switches off but logged: %s", c)
	}
	if w := dashReq(app, "PUT", "/api/admin/settings", map[string]string{"log_content": "true"}, cookie, csrf); w.Code != 200 {
		t.Fatalf("global = %d %s", w.Code, w.Body.String())
	}
	doJSON(t, h, "POST", "/v1/chat/completions", plain, chatBody(false, "via saklar global"))
	if _, c = lastLogContent(t, st, 6); !strings.Contains(c, "via saklar global") {
		t.Errorf("global switch not applied: %q", c)
	}

	// user melihat bahwa key-nya sedang dicatat
	devCookie, _ := loginAs(app, t, "dev@test", "Sandi-Kuat-123")
	w := dashReq(app, "GET", "/api/me/keys", nil, devCookie, "")
	if !strings.Contains(w.Body.String(), fmt.Sprintf(`"%d":true`, k.ID)) || !strings.Contains(w.Body.String(), `"content_retention_days":7`) {
		t.Errorf("me/keys = %s", w.Body.String())
	}
	// audit perubahan saklar
	w = dashReq(app, "GET", "/api/admin/audit", nil, cookie, "")
	if !strings.Contains(w.Body.String(), "key.log_content") || !strings.Contains(w.Body.String(), "user.log_content") {
		t.Errorf("toggle not audited")
	}
}

func TestAdminViewLogContentAndPurge(t *testing.T) {
	app, st, h, plain, k := contentEnv(t)
	st.SetKeyLogContent(k.ID, true)
	app.gate.InvalidateKey(k.KeyHash)
	doJSON(t, h, "POST", "/v1/chat/completions", plain, chatBody(false, "isi untuk admin"))
	id, _ := lastLogContent(t, st, 1)

	cookie, csrf := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	_ = csrf
	w := dashReq(app, "GET", "/api/admin/logs", nil, cookie, "")
	if !strings.Contains(w.Body.String(), `"has_content":"true"`) {
		t.Errorf("logs list must flag has_content: %s", w.Body.String())
	}
	w = dashReq(app, "GET", "/api/admin/logs/"+fmtInt(id)+"/content", nil, cookie, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "isi untuk admin") || !strings.Contains(w.Body.String(), `"user_email":"dev@test"`) ||
		!strings.Contains(w.Body.String(), `"expires_at"`) {
		t.Fatalf("content = %d %s", w.Code, w.Body.String())
	}
	if w := dashReq(app, "GET", "/api/admin/audit", nil, cookie, ""); !strings.Contains(w.Body.String(), "log.content.view") {
		t.Error("content view not audited")
	}

	// admin biasa tidak boleh membaca isi log milik super_admin
	admin, _ := st.GetUserByEmail("admin@test")
	plainA, hashA, _ := crypto.NewAPIKey()
	ka, _ := st.CreateAPIKey(admin.ID, "sa", plainA, hashA, "", "*", "", 0, 0, "")
	st.SetKeyLogContent(ka.ID, true)
	doJSON(t, h, "POST", "/v1/chat/completions", plainA, chatBody(false, "milik super admin"))
	idA, _ := lastLogContent(t, st, 2)
	st.CreateUser("admin2@test", "Kuat-2026-Pass", store.RoleAdmin)
	c2, _ := loginAs(app, t, "admin2@test", "Kuat-2026-Pass")
	if w := dashReq(app, "GET", "/api/admin/logs/"+fmtInt(idA)+"/content", nil, c2, ""); w.Code != http.StatusForbidden {
		t.Errorf("admin reading super_admin content = %d, want 403", w.Code)
	}
	if w := dashReq(app, "GET", "/api/admin/logs/"+fmtInt(id)+"/content", nil, c2, ""); w.Code != 200 {
		t.Errorf("admin reading member content = %d, want 200", w.Code)
	}

	// pengosongan setelah 7 hari: baris lama dikosongkan, baris baru tetap
	old := time.Now().Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	st.DB.Exec(`UPDATE request_logs SET ts = ? WHERE id = ?`, old, id)
	n, err := app.purgeLogContent(time.Now())
	if err != nil || n != 1 {
		t.Fatalf("purge = %d, %v; want 1", n, err)
	}
	var c string
	st.DB.QueryRow(`SELECT content_debug FROM request_logs WHERE id = ?`, id).Scan(&c)
	if c != "" {
		t.Error("old content not cleared")
	}
	st.DB.QueryRow(`SELECT content_debug FROM request_logs WHERE id = ?`, idA).Scan(&c)
	if c == "" {
		t.Error("recent content must stay")
	}
	var tokens int
	st.DB.QueryRow(`SELECT tokens_in FROM request_logs WHERE id = ?`, id).Scan(&tokens)
	if tokens == 0 {
		t.Error("metadata must stay after purge")
	}
	if w := dashReq(app, "GET", "/api/admin/logs/"+fmtInt(id)+"/content", nil, cookie, ""); w.Code != 404 || !strings.Contains(w.Body.String(), "7 hari") {
		t.Errorf("purged content = %d %s", w.Code, w.Body.String())
	}
}
