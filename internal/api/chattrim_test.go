package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jenderal/jenderalrouter/internal/translate"
)

func tmsg(role, text string) translate.Message {
	return translate.Message{Role: role, Content: []translate.Part{{Type: translate.PartText, Text: text}}}
}

func TestTrimForContextNoopWhenFits(t *testing.T) {
	msgs := []translate.Message{tmsg("system", "s"), tmsg("user", "halo")}
	out, n := trimForContext(msgs, 1000)
	if n != 0 || len(out) != 2 {
		t.Fatalf("unexpected trim: n=%d len=%d", n, len(out))
	}
}

func TestTrimForContextShrinksOldAttachmentsFirst(t *testing.T) {
	big := strings.Repeat("isi dokumen panjang. ", 2000) // ~10k token
	msgs := []translate.Message{
		tmsg("system", "s"),
		{Role: "user", Content: []translate.Part{
			{Type: translate.PartText, Text: "<lampiran nama=\"a.pdf\" jenis=\"PDF\">\n" + big + "\n</lampiran>\n\nrangkum"},
			{Type: translate.PartImage, ImageURL: "data:image/jpeg;base64,xx"},
		}},
		tmsg("assistant", "ringkasan"),
		tmsg("user", "lanjut"),
		tmsg("assistant", "ok"),
		tmsg("user", "terakhir"),
	}
	out, n := trimForContext(msgs, 2000)
	if n != 0 {
		t.Fatalf("no turn should be dropped, dropped %d", n)
	}
	if len(out) != len(msgs) {
		t.Fatalf("len = %d", len(out))
	}
	first := out[1].TextContent()
	if !strings.Contains(first, "dipangkas karena batas konteks") || !strings.Contains(first, "rangkum") || !strings.Contains(first, "1 gambar") {
		t.Errorf("old attachment not shrunk properly: %.300s", first)
	}
	for _, p := range out[1].Content {
		if p.Type == translate.PartImage {
			t.Error("old image must be removed")
		}
	}
	if len(msgs[1].Content) != 2 || !strings.Contains(msgs[1].Content[0].Text, big) {
		t.Error("input must not be mutated")
	}
}

func TestTrimForContextDropsOldestTurnsAndNotes(t *testing.T) {
	long := strings.Repeat("kata ", 2000) // ~2.5k token
	msgs := []translate.Message{tmsg("system", "dasar")}
	for i := 0; i < 6; i++ {
		msgs = append(msgs, tmsg("user", "pertanyaan ke-"+string(rune('A'+i))), tmsg("assistant", long))
	}
	msgs = append(msgs,
		translate.Message{Role: "assistant", ToolCalls: []translate.ToolCall{{ID: "c1", Name: "web_fetch", Arguments: "{}"}}},
		translate.Message{Role: "tool", ToolCallID: "c1", Content: []translate.Part{{Type: translate.PartText, Text: "hasil"}}},
	)
	msgs = append(msgs, tmsg("user", "pertanyaan terakhir"))
	out, n := trimForContext(msgs, 6000)
	if n == 0 {
		t.Fatal("expected dropped turns")
	}
	if est := estimateAll(out); est > 6000 {
		t.Errorf("still over budget: %d", est)
	}
	if out[0].Role != "system" || !strings.Contains(out[0].TextContent(), "pesan awal percakapan ini tidak disertakan") ||
		!strings.Contains(out[0].TextContent(), "pertanyaan ke-A") {
		t.Errorf("system note missing: %s", out[0].TextContent())
	}
	if out[1].Role != "user" {
		t.Errorf("history must resume at a user turn, got %s", out[1].Role)
	}
	if last := out[len(out)-1]; last.TextContent() != "pertanyaan terakhir" {
		t.Errorf("last user message changed: %q", last.TextContent())
	}
	// pasangan tool tetap utuh (tool tidak pernah jadi pesan pertama setelah system)
	for i, m := range out {
		if m.Role == "tool" && (i == 0 || out[i-1].Role != "assistant" && out[i-1].Role != "tool") {
			t.Errorf("orphan tool message at %d", i)
		}
	}
}

func TestTrimForContextTruncatesHugeLastMessage(t *testing.T) {
	huge := strings.Repeat("x", 200000) // ~50k token
	msgs := []translate.Message{tmsg("system", "dasar"), tmsg("user", huge)}
	out, _ := trimForContext(msgs, 8000)
	if est := estimateAll(out); est > 8000 {
		t.Errorf("still over budget: %d", est)
	}
	if !strings.Contains(out[1].TextContent(), "dipotong karena batas konteks") {
		t.Error("truncation note missing")
	}
	if out[0].TextContent() != "dasar" {
		t.Error("system prompt must not be truncated")
	}
}

func TestChatContextBudget(t *testing.T) {
	app, st, _ := appTestEnv(t)
	seedMockProvider(t, st, "Mock", "mk", "http://127.0.0.1:1") // context_window 100000
	steps, err := app.chatRes.Resolve("mk/mock-model")
	if err != nil {
		t.Fatal(err)
	}
	budget, window := app.chatContextBudget(steps)
	if window != 100000 || budget != 100000-8192 {
		t.Errorf("budget=%d window=%d", budget, window)
	}
	if b, w := app.chatContextBudget(nil); w != defaultContextWindow || b != defaultContextWindow-8192 {
		t.Errorf("default budget=%d window=%d", b, w)
	}
}

func TestMeChatTrimsToModelContext(t *testing.T) {
	app, st, _ := appTestEnv(t)
	var upstreamMsgs int
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []json.RawMessage `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		upstreamMsgs = len(req.Messages)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer mock.Close()
	seedMockProvider(t, st, "Mock", "mk", mock.URL)
	m, _ := st.FindModelByPublicOrAlias("mk/mock-model")
	small := 4096
	if err := st.UpdateModelFields(m.ID, nil, nil, nil, nil, &small, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	cookie, csrf := loginAs(app, t, "dev@test", "Sandi-Kuat-123")
	msgs := []map[string]string{}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, map[string]string{"role": "user", "content": "tanya"}, map[string]string{"role": "assistant", "content": strings.Repeat("jawab ", 1000)})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": "terakhir"})
	w := dashReq(app, "POST", "/api/me/chat", map[string]any{"model": "mk/mock-model", "messages": msgs}, cookie, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"context_trimmed":`) || !strings.Contains(w.Body.String(), `"context_window":4096`) {
		t.Fatalf("chat = %d %s", w.Code, w.Body.String())
	}
	if upstreamMsgs == 0 || upstreamMsgs >= len(msgs)+1 {
		t.Errorf("upstream received %d messages, want trimmed (< %d)", upstreamMsgs, len(msgs)+1)
	}
}
