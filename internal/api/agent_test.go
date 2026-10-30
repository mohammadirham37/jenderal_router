package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/translate"
)

// upstreamReq request yang diterima mock provider.
type upstreamReq struct {
	Messages []struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCallID string          `json:"tool_call_id"`
		ToolCalls  []any           `json:"tool_calls"`
	} `json:"messages"`
	Tools []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
}

func sseText(s string) string {
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": s}}}})
	return "data: " + string(b) + "\n\n"
}

func sseToolCall(id, name, args string) string {
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{
		"tool_calls": []any{map[string]any{"index": 0, "id": id, "function": map[string]any{"name": name, "arguments": args}}}}}}})
	return "data: " + string(b) + "\n\n"
}

func sseFinish(reason string) string {
	return `data: {"choices":[{"index":0,"delta":{},"finish_reason":"` + reason + `"}]}` + "\n\ndata: [DONE]\n\n"
}

// agentEnv app + mock provider yang menjawab via fungsi respond.
func agentEnv(t *testing.T, respond func(n int, req upstreamReq, w http.ResponseWriter)) (*App, *store.Store, string, string, *store.Conversation, *[]upstreamReq) {
	t.Helper()
	app, st, _ := appTestEnv(t)
	var mu sync.Mutex
	var got []upstreamReq
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req upstreamReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		got = append(got, req)
		n := len(got)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		respond(n, req, w)
	}))
	t.Cleanup(mock.Close)
	seedMockProvider(t, st, "Mock", "mk", mock.URL)
	app.search = &fakeSearcher{}
	dev, _ := st.GetUserByEmail("dev@test")
	conv, err := st.CreateConversation(dev.ID, "", "mk/mock-model", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, csrf := loginAs(app, t, "dev@test", "Sandi-Kuat-123")
	return app, st, cookie, csrf, conv, &got
}

func startRun(t *testing.T, app *App, cookie, csrf string, convID int64, body map[string]any) string {
	t.Helper()
	w := dashReq(app, "POST", "/api/me/conversations/"+fmtInt(convID)+"/runs", body, cookie, csrf)
	if w.Code != http.StatusCreated {
		t.Fatalf("start run = %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		RunID string `json:"run_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.RunID
}

// runEvents membaca SSE run sampai [END].
func runEvents(t *testing.T, app *App, cookie, runID string, from int) []map[string]any {
	t.Helper()
	w := dashReq(app, "GET", fmt.Sprintf("/api/me/runs/%s/events?from=%d", runID, from), nil, cookie, "")
	if !strings.Contains(w.Body.String(), "data: [END]") {
		t.Fatalf("stream not finished: %s", w.Body.String())
	}
	var evs []map[string]any
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if p, ok := strings.CutPrefix(line, "data: "); ok && p != "[END]" {
			var ev map[string]any
			json.Unmarshal([]byte(p), &ev)
			evs = append(evs, ev)
		}
	}
	return evs
}

func TestAgentRunWithToolSavesHistory(t *testing.T) {
	app, st, cookie, csrf, conv, got := agentEnv(t, func(n int, req upstreamReq, w http.ResponseWriter) {
		if n == 1 {
			fmt.Fprint(w, sseText("Saya cari dulu."), sseToolCall("c1", "web_search", `{"query":"rilis go"}`), sseFinish("tool_calls"))
			return
		}
		fmt.Fprint(w, sseText("Go 1.30 sudah rilis [Go](https://go.dev/doc/go1.30)."), sseFinish("stop"))
	})
	runID := startRun(t, app, cookie, csrf, conv.ID, map[string]any{"model": "mk/mock-model", "content": "rilis go terbaru?"})
	evs := runEvents(t, app, cookie, runID, 0)

	types := []string{}
	for _, e := range evs {
		types = append(types, e["t"].(string))
	}
	joined := strings.Join(types, ",")
	if !strings.Contains(joined, "tool,tool_end") || types[len(types)-1] != "done" {
		t.Fatalf("event order = %s", joined)
	}
	if len(*got) != 2 {
		t.Fatalf("upstream calls = %d, want 2", len(*got))
	}
	second := (*got)[1]
	last := second.Messages[len(second.Messages)-1]
	if last.Role != "tool" || last.ToolCallID != "c1" || !strings.Contains(string(last.Content), "go.dev/doc/go1.30") {
		t.Errorf("tool result not sent back to model: %+v", last)
	}
	tools := []string{}
	for _, tl := range (*got)[0].Tools {
		tools = append(tools, tl.Function.Name)
	}
	if strings.Join(tools, ",") != "web_search,web_fetch" {
		t.Errorf("tools offered = %v (create_document must need doc_prompts)", tools)
	}

	msgs, _ := st.ListMessages(conv.ID)
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("saved messages = %+v", msgs)
	}
	if want := "Saya cari dulu.\n\nGo 1.30 sudah rilis"; !strings.HasPrefix(msgs[1].Content, want) {
		t.Errorf("assistant content = %q", msgs[1].Content)
	}
	meta := parseMeta(msgs[1].Meta)
	if len(meta.Tools) != 1 || meta.Tools[0].Status != "ok" || meta.Tools[0].Hits == nil || *meta.Tools[0].Hits != 1 {
		t.Errorf("tool meta = %+v", meta.Tools)
	}
	// replay dari tengah hanya mengirim sisa event
	if tail := runEvents(t, app, cookie, runID, len(evs)-1); len(tail) != 1 || tail[0]["t"] != "done" {
		t.Errorf("replay from N = %+v", tail)
	}

	// giliran berikutnya: riwayat dari DB membawa tool_calls + hasil
	runID = startRun(t, app, cookie, csrf, conv.ID, map[string]any{"model": "mk/mock-model", "content": "lanjut"})
	runEvents(t, app, cookie, runID, 0)
	third := (*got)[2]
	roles := []string{}
	for _, m := range third.Messages {
		roles = append(roles, m.Role)
	}
	if strings.Join(roles, ",") != "system,user,assistant,tool,assistant,user" {
		t.Errorf("history roles = %v", roles)
	}
}

func TestAgentRunCreateDocument(t *testing.T) {
	app, st, cookie, csrf, conv, _ := agentEnv(t, func(n int, req upstreamReq, w http.ResponseWriter) {
		if n == 1 {
			fmt.Fprint(w, sseText("Baik, saya siapkan."), sseToolCall("d1", "create_document", `{"type":"xlsx","title":"Data"}`), sseFinish("tool_calls"))
			return
		}
		fmt.Fprint(w, sseText("# Data Penduduk ✓\n\n| A | B |\n|---|---|\n| 1 | 2 |"), sseFinish("stop"))
	})
	runID := startRun(t, app, cookie, csrf, conv.ID, map[string]any{
		"model": "mk/mock-model", "content": "data penduduk, saya mau unduh", "doc_tool": true,
		"doc_prompts": map[string]string{"xlsx": "FORMAT EXCEL", "docx": "FORMAT WORD"},
	})
	evs := runEvents(t, app, cookie, runID, 0)
	done := evs[len(evs)-1]
	doc, _ := done["doc"].(map[string]any)
	if doc == nil || doc["type"] != "xlsx" || doc["title"] != "Data" {
		t.Fatalf("done doc = %+v", done)
	}
	msgs, _ := st.ListMessages(conv.ID)
	content := msgs[1].Content
	start := int(doc["start"].(float64))
	if !strings.HasPrefix(content[start:], "# Data Penduduk") {
		t.Errorf("doc start %d points at %q", start, content[start:])
	}
}

func TestAgentRunCancelAndConflict(t *testing.T) {
	release := make(chan struct{})
	app, st, cookie, csrf, conv, _ := agentEnv(t, func(n int, req upstreamReq, w http.ResponseWriter) {
		fmt.Fprint(w, sseText("mulai "))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-time.After(5 * time.Second):
		}
		fmt.Fprint(w, sseText("selesai"), sseFinish("stop"))
	})
	defer close(release)
	runID := startRun(t, app, cookie, csrf, conv.ID, map[string]any{"model": "mk/mock-model", "content": "halo"})

	if w := dashReq(app, "POST", "/api/me/conversations/"+fmtInt(conv.ID)+"/runs", map[string]any{"model": "mk/mock-model", "content": "lagi"}, cookie, csrf); w.Code != http.StatusConflict {
		t.Errorf("second run = %d, want 409", w.Code)
	}
	w := dashReq(app, "GET", "/api/me/conversations/"+fmtInt(conv.ID), nil, cookie, "")
	if !strings.Contains(w.Body.String(), `"active_run":"`+runID+`"`) {
		t.Errorf("active_run missing: %s", w.Body.String())
	}
	if w := dashReq(app, "DELETE", "/api/me/conversations/"+fmtInt(conv.ID)+"/messages?from=1", nil, cookie, csrf); w.Code != http.StatusConflict {
		t.Errorf("truncate during run = %d, want 409", w.Code)
	}
	time.Sleep(100 * time.Millisecond)
	if w := dashReq(app, "POST", "/api/me/runs/"+runID+"/cancel", nil, cookie, csrf); w.Code != 200 {
		t.Fatalf("cancel = %d", w.Code)
	}
	evs := runEvents(t, app, cookie, runID, 0)
	if done := evs[len(evs)-1]; done["t"] != "done" || done["stopped"] != true {
		t.Fatalf("done = %+v", done)
	}
	msgs, _ := st.ListMessages(conv.ID)
	if len(msgs) != 2 || !parseMeta(msgs[1].Meta).Stopped {
		t.Errorf("stopped answer not saved: %+v", msgs)
	}

	// run milik user lain tidak bisa diakses
	c2, _ := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	if w := dashReq(app, "GET", "/api/me/runs/"+runID+"/events", nil, c2, ""); w.Code != 404 {
		t.Errorf("foreign run events = %d, want 404", w.Code)
	}
}

func TestHistoryFromMessages(t *testing.T) {
	hits := 1
	toolMeta, _ := json.Marshal(messageMeta{Tools: []toolUse{
		{ID: "a", Name: "web_search", Args: `{"query":"x"}`, Round: 0, Status: "ok", Result: "hasil", Hits: &hits},
		{ID: "b", Name: "web_fetch", Round: 1, Status: "err", Result: "ERROR: x"},
	}})
	imgMeta, _ := json.Marshal(messageMeta{Attachments: []attachment{
		{Name: "a.pdf", Kind: "pdf", Text: "isi pdf", Pages: 2},
		{Name: "g.jpg", Kind: "image", DataURL: "data:image/jpeg;base64,AA"},
	}})
	msgs := []*store.Message{
		{Role: "user", Content: "lihat", Meta: imgMeta},
		{Role: "assistant", Content: "jawab", Meta: toolMeta},
		{Role: "user", Content: "lagi"},
	}
	h := historyFromMessages(msgs)
	roles := []string{}
	for _, m := range h {
		roles = append(roles, m.Role)
	}
	if strings.Join(roles, ",") != "user,assistant,tool,assistant,tool,assistant,user" {
		t.Fatalf("roles = %v", roles)
	}
	first := h[0]
	if len(first.Content) != 2 || first.Content[1].Type != translate.PartImage ||
		!strings.Contains(first.Content[0].Text, `<lampiran nama="a.pdf" jenis="PDF" halaman="2">`) ||
		!strings.HasSuffix(first.Content[0].Text, "lihat") {
		t.Errorf("user message = %+v", first.Content)
	}
	if h[1].ToolCalls[0].ID != "a" || h[3].ToolCalls[0].Arguments != "{}" {
		t.Errorf("tool calls = %+v / %+v", h[1].ToolCalls, h[3].ToolCalls)
	}
}
