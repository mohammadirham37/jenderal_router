package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jenderal/jenderalrouter/internal/config"
	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/translate"
)

func TestTanggalID(t *testing.T) {
	loc := time.FixedZone("WIB", 7*3600)
	got := tanggalID(time.Date(2026, 10, 29, 9, 5, 0, 0, loc))
	if want := "Kamis, 29 Oktober 2026 09:05 WIB"; got != want {
		t.Errorf("tanggalID = %q, want %q", got, want)
	}
}

func TestChatSystemPrompt(t *testing.T) {
	a := &App{cfg: &config.Config{TZ: "Asia/Jakarta"}}
	u := &store.User{Email: "budi@test"}

	plain := a.chatSystemPrompt(u, nil, "")
	if !strings.Contains(plain, "budi@test") || !strings.Contains(plain, "Waktu sekarang:") {
		t.Errorf("prompt missing user or date:\n%s", plain)
	}
	if strings.Contains(plain, "Tool tersedia") || strings.Contains(plain, "Instruksi khusus") {
		t.Errorf("prompt without tools/custom must not mention them:\n%s", plain)
	}

	full := a.chatSystemPrompt(u, []translate.Tool{{Name: "web_fetch"}}, "Jawab dalam bahasa Inggris.")
	if !strings.Contains(full, "Tool tersedia: web_fetch") {
		t.Errorf("prompt missing tool list:\n%s", full)
	}
	if !strings.Contains(full, "web_fetch membaca") || strings.Contains(full, "create_document menyiapkan") || !strings.Contains(full, "tautan Markdown") {
		t.Errorf("prompt must describe only the tools sent:\n%s", full)
	}
	if !strings.Contains(full, "Jawab dalam bahasa Inggris.") {
		t.Errorf("prompt missing custom instructions:\n%s", full)
	}
}

func TestWithChatSystemPromptMergesLeadingSystem(t *testing.T) {
	text := func(s string) []translate.Part { return []translate.Part{{Type: translate.PartText, Text: s}} }
	msgs := []translate.Message{
		{Role: translate.RoleSystem, Content: text("Buat file Word.")},
		{Role: translate.RoleUser, Content: text("halo")},
	}
	got := withChatSystemPrompt(msgs, "DASAR")
	if len(got) != 2 {
		t.Fatalf("want 2 messages, got %d", len(got))
	}
	if got[0].Role != translate.RoleSystem || got[0].TextContent() != "DASAR\n\nBuat file Word." {
		t.Errorf("system = %q", got[0].TextContent())
	}
	if got[1].Role != translate.RoleUser {
		t.Errorf("second message role = %s, want user", got[1].Role)
	}
}

// Chat dashboard: upstream menerima tepat satu system message di depan
// (prompt dasar + instruksi percakapan + system klien), dan riwayat tool
// (assistant tool_calls + role tool) diteruskan utuh.
func TestMeChatSendsSystemPromptAndToolHistory(t *testing.T) {
	app, st, _ := appTestEnv(t)
	var got struct {
		Messages []struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			ToolCallID string          `json:"tool_call_id"`
			ToolCalls  []any           `json:"tool_calls"`
		} `json:"messages"`
	}
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
	}))
	defer mock.Close()
	seedMockProvider(t, st, "Mock", "mk", mock.URL)
	dev, _ := st.GetUserByEmail("dev@test")
	conv, err := st.CreateConversation(dev.ID, "", "mk/mock-model", "Selalu jawab dengan pantun.", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, csrf := loginAs(app, t, "dev@test", "Sandi-Kuat-123")
	w := dashReq(app, "POST", "/api/me/chat", map[string]any{
		"model": "mk/mock-model", "stream": false, "conversation_id": conv.ID,
		"tools": []map[string]any{{"type": "function", "function": map[string]any{"name": "web_fetch", "parameters": map[string]any{"type": "object"}}}},
		"messages": []map[string]any{
			{"role": "system", "content": "Buat file Word."},
			{"role": "user", "content": "ringkas go.dev"},
			{"role": "assistant", "content": nil, "tool_calls": []map[string]any{{"id": "c1", "type": "function", "function": map[string]any{"name": "web_fetch", "arguments": `{"url":"https://go.dev"}`}}}},
			{"role": "tool", "tool_call_id": "c1", "content": "Judul: Go"},
			{"role": "assistant", "content": "Go adalah bahasa."},
			{"role": "user", "content": "lanjut"},
		},
	}, cookie, csrf)
	if w.Code != 200 {
		t.Fatalf("chat = %d %s", w.Code, w.Body.String())
	}
	if len(got.Messages) != 6 {
		t.Fatalf("upstream messages = %d, want 6: %+v", len(got.Messages), got.Messages)
	}
	sys := string(got.Messages[0].Content)
	if got.Messages[0].Role != "system" || !strings.Contains(sys, "Waktu sekarang") ||
		!strings.Contains(sys, "pantun") || !strings.Contains(sys, "Buat file Word.") || !strings.Contains(sys, "web_fetch") {
		t.Errorf("system message = %s", sys)
	}
	if got.Messages[1].Role != "user" {
		t.Errorf("only one system message expected, second role = %s", got.Messages[1].Role)
	}
	if len(got.Messages[2].ToolCalls) != 1 || got.Messages[3].Role != "tool" || got.Messages[3].ToolCallID != "c1" {
		t.Errorf("tool history broken: %+v", got.Messages[2:4])
	}

	// meta tersimpan dan ikut kembali saat percakapan dibuka
	w = dashReq(app, "POST", "/api/me/conversations/"+fmtInt(conv.ID)+"/messages", map[string]any{
		"role": "assistant", "content": "", "meta": map[string]any{"stopped": true, "tools": []any{map[string]any{"id": "c1", "result": "Judul: Go"}}},
	}, cookie, csrf)
	if w.Code != 200 {
		t.Fatalf("save meta = %d %s", w.Code, w.Body.String())
	}
	w = dashReq(app, "GET", "/api/me/conversations/"+fmtInt(conv.ID), nil, cookie, "")
	if !strings.Contains(w.Body.String(), `"meta":{"stopped":true`) {
		t.Fatalf("meta tidak kembali: %s", w.Body.String())
	}
	w = dashReq(app, "POST", "/api/me/conversations/"+fmtInt(conv.ID)+"/messages", map[string]any{"role": "assistant", "content": ""}, cookie, csrf)
	if w.Code != 400 {
		t.Fatalf("pesan kosong tanpa meta harus 400: %d", w.Code)
	}
}
