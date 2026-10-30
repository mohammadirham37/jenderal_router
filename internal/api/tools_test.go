package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jenderal/jenderalrouter/internal/websearch"
)

type fakeSearcher struct {
	got string
	err error
}

func (f *fakeSearcher) Name() string { return "fake" }
func (f *fakeSearcher) Search(_ context.Context, q string, n int) ([]websearch.Result, error) {
	f.got = q
	if f.err != nil {
		return nil, f.err
	}
	return []websearch.Result{{Title: "Go 1.30", URL: "https://go.dev/doc/go1.30", Snippet: "rilis"}}[:min(n, 1)], nil
}

func TestToolWebSearch(t *testing.T) {
	app, _, _ := appTestEnv(t)
	fs := &fakeSearcher{}
	app.search = fs
	cookie, csrf := loginAs(app, t, "dev@test", "Sandi-Kuat-123")

	w := dashReq(app, "POST", "/api/me/tools/websearch", map[string]any{"query": "  rilis go terbaru "}, cookie, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"url":"https://go.dev/doc/go1.30"`) || !strings.Contains(w.Body.String(), `"backend":"fake"`) {
		t.Fatalf("search = %d %s", w.Code, w.Body.String())
	}
	if fs.got != "rilis go terbaru" {
		t.Errorf("query not trimmed: %q", fs.got)
	}
	if w := dashReq(app, "POST", "/api/me/tools/websearch", map[string]any{"query": " "}, cookie, csrf); w.Code != 400 {
		t.Errorf("empty query = %d, want 400", w.Code)
	}
	fs.err = errors.New("HTTP 429")
	if w := dashReq(app, "POST", "/api/me/tools/websearch", map[string]any{"query": "x"}, cookie, csrf); w.Code != 502 || !strings.Contains(w.Body.String(), "pencarian gagal") {
		t.Errorf("backend error = %d %s", w.Code, w.Body.String())
	}
	// tanpa CSRF ditolak (endpoint mutasi)
	if w := dashReq(app, "POST", "/api/me/tools/websearch", map[string]any{"query": "x"}, cookie, ""); w.Code == 200 {
		t.Errorf("missing CSRF accepted")
	}
}

func TestMeModelsVisionAndChatBodyLimit(t *testing.T) {
	app, st, _ := appTestEnv(t)
	seedMockProvider(t, st, "Mock", "mk", "http://127.0.0.1:1") // model mock: Vision=true
	cookie, csrf := loginAs(app, t, "dev@test", "Sandi-Kuat-123")
	if w := dashReq(app, "GET", "/api/me/models", nil, cookie, ""); !strings.Contains(w.Body.String(), `"vision":true`) {
		t.Fatalf("models harus memuat flag vision: %s", w.Body.String())
	}
	big := strings.Repeat("a", maxChatBody)
	w := dashReq(app, "POST", "/api/me/chat", map[string]any{
		"model": "mk/mock-model", "messages": []map[string]string{{"role": "user", "content": big}},
	}, cookie, csrf)
	if w.Code != 413 {
		t.Fatalf("body > batas harus 413, dapat %d %.200s", w.Code, w.Body.String())
	}
}

func TestTruncateMessages(t *testing.T) {
	app, st, _ := appTestEnv(t)
	cookie, csrf := loginAs(app, t, "dev@test", "Sandi-Kuat-123")
	dev, _ := st.GetUserByEmail("dev@test")
	c, _ := st.CreateConversation(dev.ID, "", "m", "", nil, nil)
	base := "/api/me/conversations/" + fmtInt(c.ID) + "/messages"
	ids := []int64{}
	for _, role := range []string{"user", "assistant", "user", "assistant"} {
		w := dashReq(app, "POST", base, map[string]any{"role": role, "content": role + " x"}, cookie, csrf)
		var resp struct {
			ID int64 `json:"id"`
		}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if w.Code != 200 || resp.ID == 0 {
			t.Fatalf("save = %d %s", w.Code, w.Body.String())
		}
		ids = append(ids, resp.ID)
	}
	if w := dashReq(app, "DELETE", base+"?from="+fmtInt(ids[2]), nil, cookie, csrf); w.Code != 200 || !strings.Contains(w.Body.String(), `"deleted":2`) {
		t.Fatalf("truncate = %d %s", w.Code, w.Body.String())
	}
	if msgs, _ := st.ListMessages(c.ID); len(msgs) != 2 {
		t.Fatalf("remaining = %d, want 2", len(msgs))
	}
	if w := dashReq(app, "DELETE", base, nil, cookie, csrf); w.Code != 400 {
		t.Errorf("missing from = %d, want 400", w.Code)
	}
	// percakapan milik user lain → 404
	admin, _ := st.GetUserByEmail("admin@test")
	other, _ := st.CreateConversation(admin.ID, "", "m", "", nil, nil)
	st.AddMessage(other.ID, "user", "rahasia", "", "")
	if w := dashReq(app, "DELETE", "/api/me/conversations/"+fmtInt(other.ID)+"/messages?from=1", nil, cookie, csrf); w.Code != 404 {
		t.Errorf("other user's conversation = %d, want 404", w.Code)
	}
	if msgs, _ := st.ListMessages(other.ID); len(msgs) != 1 {
		t.Error("other user's messages must stay")
	}
}
