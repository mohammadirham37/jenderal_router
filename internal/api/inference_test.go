package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jenderal/jenderalrouter/internal/config"
	"github.com/jenderal/jenderalrouter/internal/crypto"
	"github.com/jenderal/jenderalrouter/internal/store"
)

// appTestEnv merakit App penuh dengan DB in-memory + super admin + key.
func appTestEnv(t *testing.T) (*App, *store.Store, string) {
	t.Helper()
	store.SetMasterKey(bytes.Repeat([]byte{9}, 32))
	dir := t.TempDir()
	cfg := &config.Config{
		Addr: "127.0.0.1:0", DataDir: dir,
		DatabaseURL: fmt.Sprintf("file:%s/test.db?cache=shared", dir),
		MasterKey:   bytes.Repeat([]byte{9}, 32),
		TZ:          "Asia/Jakarta", SessionTTL: time.Hour, BackupEnabled: false,
		LLamastashURL: "http://127.0.0.1:11435/v1",
	}
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	logins.reset()

	// super admin
	admin, err := app.st.CreateUser("admin@test", "Sandi-Kuat-123", store.RoleSuperAdmin)
	if err != nil {
		t.Fatal(err)
	}
	// user + key
	user, err := app.st.CreateUser("dev@test", "Sandi-Kuat-123", store.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	plain, hash, _ := crypto.NewAPIKey()
	if _, err := app.st.CreateAPIKey(user.ID, "utama", plain, hash, "", "*", "", 0, 0, ""); err != nil {
		t.Fatal(err)
	}
	_ = admin
	return app, app.st, plain
}

// seedMockProvider mendaftarkan provider openai-compatible menunjuk mock URL.
func seedMockProvider(t *testing.T, st *store.Store, name, prefix, url string) *store.Provider {
	t.Helper()
	p, err := st.CreateProvider(store.ProviderOpenAICompat, name, prefix, url, store.CredentialSettings{Strategy: store.CredStrategyRoundRobin, SSRFAllowPrivate: true}, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddCredential(p.ID, "mock1", "sk-mock-"+prefix+"-1", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddCredential(p.ID, "mock2", "sk-mock-"+prefix+"-2", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateModel(p.ID, "mock-model", "", "Mock", 1.5, 6, 100000, store.ModelCap{Tools: true, Vision: true}, true); err != nil {
		t.Fatal(err)
	}
	return p
}

func inferBody(model string, stream bool) []byte {
	b, _ := json.Marshal(map[string]any{
		"model":    model,
		"messages": []map[string]any{{"role": "user", "content": "Halo"}},
		"stream":   stream,
	})
	return b
}

func doJSON(t *testing.T, h http.Handler, method, path, key string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if key != "" {
		r.Header.Set("Authorization", "Bearer "+key)
	}
	r.RemoteAddr = "203.0.113.5:4444"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestChatCompletionsNonStreamE2E(t *testing.T) {
	app, st, key := appTestEnv(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("upstream path = %s", r.URL.Path)
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "mock-model" {
			t.Errorf("upstream model = %v (harus nama upstream)", req["model"])
		}
		fmt.Fprint(w, `{"id":"cmpl-1","model":"mock-model","choices":[{"index":0,"message":{"role":"assistant","content":"Halo dari mock"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":4}}`)
	}))
	defer mock.Close()
	seedMockProvider(t, st, "Mock", "mk", mock.URL)

	w := doJSON(t, app.Handler(), "POST", "/v1/chat/completions", key, inferBody("mk/mock-model", false))
	if w.Code != 200 {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["object"] != "chat.completion" {
		t.Fatalf("resp = %v", resp)
	}
	if w.Header().Get("X-Route-Provider") != "Mock" || w.Header().Get("X-Route-Model") != "mk/mock-model" {
		t.Errorf("X-Route headers = %s %s", w.Header().Get("X-Route-Provider"), w.Header().Get("X-Route-Model"))
	}
	// tunggu recorder flush
	app.rec.Close()
	var tokensIn, tokensOut int64
	if err := st.DB.QueryRow(`SELECT tokens_in, tokens_out FROM request_logs ORDER BY id DESC LIMIT 1`).Scan(&tokensIn, &tokensOut); err != nil {
		t.Fatalf("log belum ditulis: %v", err)
	}
	if tokensIn != 9 || tokensOut != 4 {
		t.Errorf("tokens = %d/%d", tokensIn, tokensOut)
	}
}

func TestChatCompletionsStreamE2E(t *testing.T) {
	app, st, key := appTestEnv(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for _, data := range []string{
			`{"id":"c1","model":"mock-model","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
			`{"id":"c1","model":"mock-model","choices":[{"index":0,"delta":{"content":"Sa"}}]}`,
			`{"id":"c1","model":"mock-model","choices":[{"index":0,"delta":{"content":"lon"}}]}`,
			`{"id":"c1","model":"mock-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`{"id":"c1","model":"mock-model","choices":[],"usage":{"prompt_tokens":6,"completion_tokens":2}}`,
			`[DONE]`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", data)
			f.Flush()
		}
	}))
	defer mock.Close()
	seedMockProvider(t, st, "Mock", "mk", mock.URL)

	w := doJSON(t, app.Handler(), "POST", "/v1/chat/completions", key, inferBody("mk/mock-model", true))
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"content":"Sa"`) || !strings.Contains(body, `"content":"lon"`) {
		t.Errorf("stream chunks hilang:\n%s", body)
	}
	if !strings.Contains(body, `"finish_reason":"stop"`) || !strings.Contains(body, "[DONE]") {
		t.Errorf("penutup stream hilang:\n%s", body)
	}
	if !strings.Contains(body, `"total_tokens":8`) {
		t.Errorf("chunk usage hilang:\n%s", body)
	}
	if w.Header().Get("X-Route-Model") != "mk/mock-model" {
		t.Errorf("header route = %s", w.Header().Get("X-Route-Model"))
	}
}

func TestFallbackOn429E2E(t *testing.T) {
	app, st, key := appTestEnv(t)
	var primaryHits, secondaryHits int
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits++
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"rate limited","type":"rate_limit_error"}}`)
	}))
	defer primary.Close()
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryHits++
		fmt.Fprint(w, `{"id":"c2","model":"backup","choices":[{"index":0,"message":{"role":"assistant","content":"dari backup"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3}}`)
	}))
	defer secondary.Close()

	p1 := seedMockProvider(t, st, "Primer", "p1", primary.URL)
	p2 := seedMockProvider(t, st, "Cadangan", "p2", secondary.URL)

	// combo: primer → cadangan
	if _, err := st.CreateCombo("combo-uji", "uji fallback", []int64{
		modelIDByPrefix(t, st, p1.ID), modelIDByPrefix(t, st, p2.ID),
	}); err != nil {
		t.Fatal(err)
	}

	w := doJSON(t, app.Handler(), "POST", "/v1/chat/completions", key, inferBody("combo-uji", false))
	if w.Code != 200 {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Route-Provider") != "Cadangan" {
		t.Errorf("harus dijawab Cadangan, dapat %s", w.Header().Get("X-Route-Provider"))
	}
	if w.Header().Get("X-Route-Attempts") != "3" { // 2x primer (retry) + 1x cadangan
		t.Errorf("attempts = %s", w.Header().Get("X-Route-Attempts"))
	}
	if primaryHits != 2 || secondaryHits != 1 {
		t.Errorf("hits primer=%d cadangan=%d", primaryHits, secondaryHits)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	choices := resp["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "dari backup" {
		t.Errorf("content = %v", msg["content"])
	}
	// error attempts rinci tersedia? (sukses → tidak ada error body)
}

func TestAllProvidersDownE2E(t *testing.T) {
	app, st, key := appTestEnv(t)
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"message":"boom"}}`)
	}))
	defer dead.Close()
	p := seedMockProvider(t, st, "Mati", "dd", dead.URL)
	_ = p

	w := doJSON(t, app.Handler(), "POST", "/v1/chat/completions", key, inferBody("dd/mock-model", false))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, mau 502", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]any)
	if errObj["code"] != "upstream_error" {
		t.Errorf("code = %v", errObj["code"])
	}
	// attempts tercatat: 2 (retry)
	attempts := errObj["attempts"].([]any)
	if len(attempts) != 2 {
		t.Errorf("attempts = %d", len(attempts))
	}
}

func TestMessagesEndpointAnthropicE2E(t *testing.T) {
	app, st, key := appTestEnv(t)
	// mock upstream anthropic-compatible
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("anthropic-version header hilang")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","model":"mock-an",
			"content":[{"type":"text","text":"Balasan Anthropic"}],
			"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	defer mock.Close()

	p, err := st.CreateProvider(store.ProviderAnthropic, "AnMock", "am", mock.URL, store.CredentialSettings{SSRFAllowPrivate: true}, true)
	if err != nil {
		t.Fatal(err)
	}
	st.AddCredential(p.ID, "k", "an-key", 1)
	st.CreateModel(p.ID, "mock-an", "", "", 3, 15, 200000, store.ModelCap{Tools: true, Vision: true}, true)

	body := []byte(`{"model":"am/mock-an","max_tokens":64,"messages":[{"role":"user","content":"Halo"}]}`)
	r := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
	r.Header.Set("x-api-key", key)
	r.Header.Set("anthropic-version", "2023-06-01")
	r.RemoteAddr = "203.0.113.5:4444"
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["type"] != "message" || resp["stop_reason"] != "end_turn" {
		t.Fatalf("resp = %v", resp)
	}
	content := resp["content"].([]any)
	if content[0].(map[string]any)["text"] != "Balasan Anthropic" {
		t.Errorf("content = %v", content)
	}
}

func TestMessagesStreamAnthropicE2E(t *testing.T) {
	app, st, key := appTestEnv(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		frames := []string{
			`{"type":"message_start","message":{"id":"msg_9","model":"m","usage":{"input_tokens":4,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hai"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
			`{"type":"message_stop"}`,
		}
		for _, fr := range frames {
			fmt.Fprintf(w, "data: %s\n\n", fr)
			f.Flush()
		}
	}))
	defer mock.Close()
	p, _ := st.CreateProvider(store.ProviderAnthropic, "AnMock2", "am2", mock.URL, store.CredentialSettings{SSRFAllowPrivate: true}, true)
	st.AddCredential(p.ID, "k", "an", 1)
	st.CreateModel(p.ID, "mock-an", "", "", 0, 0, 0, store.ModelCap{}, true)

	body := []byte(`{"model":"am2/mock-an","max_tokens":32,"stream":true,"messages":[{"role":"user","content":"Hai"}]}`)
	r := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
	r.Header.Set("x-api-key", key)
	r.RemoteAddr = "1.2.3.4:5"
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, r)
	out := w.Body.String()
	for _, want := range []string{"event: message_start", "event: content_block_delta", `"text":"Hai"`, "event: message_stop", `"stop_reason":"end_turn"`} {
		if !strings.Contains(out, want) {
			t.Errorf("frame %q hilang:\n%s", want, out)
		}
	}
}

func TestAuthErrorsFormatE2E(t *testing.T) {
	app, st, key := appTestEnv(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer mock.Close()
	seedMockProvider(t, st, "M", "mm", mock.URL)

	// tanpa key → 401 openai-style
	w := doJSON(t, app.Handler(), "POST", "/v1/chat/completions", "", inferBody("mm/mock-model", false))
	if w.Code != 401 {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_api_key") {
		t.Errorf("body = %s", w.Body.String())
	}

	// model tidak diizinkan → 403
	plain, hash, _ := crypto.NewAPIKey()
	user, _ := st.GetUserByEmail("dev@test")
	st.CreateAPIKey(user.ID, "terbatas", plain, hash, "", "mk/mock-model", "", 0, 0, "")
	w = doJSON(t, app.Handler(), "POST", "/v1/chat/completions", plain, inferBody("model-lain", false))
	if w.Code != 403 {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "model_not_allowed") {
		t.Errorf("body = %s", w.Body.String())
	}

	// model tidak ada → 404
	w = doJSON(t, app.Handler(), "POST", "/v1/chat/completions", key, inferBody("tak/ada", false))
	if w.Code != 404 || !strings.Contains(w.Body.String(), "model_not_found") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	// format anthropic untuk /v1/messages
	w = doJSON(t, app.Handler(), "POST", "/v1/messages", "", inferBody("mm/mock-model", false))
	if !strings.Contains(w.Body.String(), `"type":"error"`) {
		t.Errorf("error harus format anthropic: %s", w.Body.String())
	}
}

func TestQuotaEnforcementE2E(t *testing.T) {
	app, st, key := appTestEnv(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer mock.Close()
	p := seedMockProvider(t, st, "Q", "qq", mock.URL)
	_ = p
	user, _ := st.GetUserByEmail("dev@test")
	st.PutQuota("user", user.ID, "day", 100, 0, 0)
	st.RecordQuotaUsage("user", user.ID, 100, 0)

	w := doJSON(t, app.Handler(), "POST", "/v1/chat/completions", key, inferBody("qq/mock-model", false))
	if w.Code != 429 {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "quota_exceeded") {
		t.Errorf("body = %s", w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header harus ada")
	}
}

func TestModelsEndpointE2E(t *testing.T) {
	app, st, key := appTestEnv(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer mock.Close()
	seedMockProvider(t, st, "Mock", "mk", mock.URL)

	w := doJSON(t, app.Handler(), "GET", "/v1/models", key, nil)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("models = %d", len(data))
	}
	if data[0].(map[string]any)["id"] != "mk/mock-model" {
		t.Errorf("model id = %v", data[0])
	}
}

func TestUsageMeE2E(t *testing.T) {
	app, st, key := appTestEnv(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer mock.Close()
	seedMockProvider(t, st, "Mock", "mk", mock.URL)
	user, _ := st.GetUserByEmail("dev@test")
	st.PutQuota("user", user.ID, "day", 100000, 0, 5)

	w := doJSON(t, app.Handler(), "GET", "/v1/usage/me", key, nil)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["quotas"] == nil || resp["today"] == nil {
		t.Fatalf("resp = %v", resp)
	}
}

func TestRPMLimitE2E(t *testing.T) {
	app, st, _ := appTestEnv(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer mock.Close()
	seedMockProvider(t, st, "Mock", "mk", mock.URL)
	user, _ := st.GetUserByEmail("dev@test")
	plain, hash, _ := crypto.NewAPIKey()
	st.CreateAPIKey(user.ID, "rpm", plain, hash, "", "*", "", 2, 0, "") // RPM=2

	var sawLimit bool
	for i := 0; i < 4; i++ {
		w := doJSON(t, app.Handler(), "POST", "/v1/chat/completions", plain, inferBody("mk/mock-model", false))
		if w.Code == 429 && strings.Contains(w.Body.String(), "rate_limited") {
			sawLimit = true
			break
		}
	}
	if !sawLimit {
		t.Fatal("RPM=2 harus terlampaui pada request ke-3")
	}
}

func modelIDByPrefix(t *testing.T, st *store.Store, providerID int64) int64 {
	t.Helper()
	models, err := st.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range models {
		if m.ProviderID == providerID {
			return m.ID
		}
	}
	t.Fatal("model tidak ditemukan")
	return 0
}
