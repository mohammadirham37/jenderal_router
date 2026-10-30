package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/translate"
	"github.com/jenderal/jenderalrouter/internal/usage"
)

// Agen chat server-side: loop model ↔ tool berjalan di server sehingga
// tugas panjang (riset beberapa halaman, dokumen) tetap selesai walau tab
// browser ditutup. Hasil disimpan ke riwayat saat run selesai.

const maxAgentRounds = 10

var docTypes = map[string]bool{"docx": true, "pptx": true, "xlsx": true, "csv": true}

func toolDef(name, desc, params string) translate.Tool {
	return translate.Tool{Name: name, Description: desc, Parameters: json.RawMessage(params)}
}

var (
	webSearchTool = toolDef("web_search",
		"Mencari di web dan mengembalikan daftar hasil (judul, URL, cuplikan). Gunakan untuk informasi terkini atau fakta yang tidak diketahui pasti.",
		`{"type":"object","properties":{"query":{"type":"string","description":"Kueri pencarian singkat dan spesifik"}},"required":["query"]}`)
	webFetchTool = toolDef("web_fetch",
		"Mengambil isi teks sebuah halaman web publik. Gunakan saat pengguna menyebutkan URL, atau untuk membaca hasil web_search yang relevan.",
		`{"type":"object","properties":{"url":{"type":"string","description":"URL lengkap halaman, mulai dengan http atau https"}},"required":["url"]}`)
	createDocTool = toolDef("create_document",
		"Menyiapkan file unduhan dari jawaban berikutnya. Panggil hanya bila pengguna meminta file Word/PowerPoint/Excel/CSV, dokumen, laporan, presentasi, atau spreadsheet.",
		`{"type":"object","properties":{"type":{"type":"string","enum":["docx","pptx","xlsx","csv"],"description":"docx=Word, pptx=PowerPoint, xlsx=Excel, csv=CSV"},"title":{"type":"string","description":"Judul dokumen"}},"required":["type"]}`)
)

// runParams permintaan run dari UI.
type runParams struct {
	Model       string            `json:"model"`
	Content     string            `json:"content"`
	Attachments []attachment      `json:"attachments"`
	ForcedDoc   string            `json:"forced_doc"`  // docx|pptx|xlsx|csv: instruksi dokumen sejak awal
	DocPrompts  map[string]string `json:"doc_prompts"` // instruksi format per jenis dokumen (milik generator UI)
	DocTool     bool              `json:"doc_tool"`    // tawarkan create_document ke model
}

// agentState keadaan satu giliran assistant yang sedang dirakit.
type agentState struct {
	content   strings.Builder
	reasoning strings.Builder
	tools     []*toolUse
	model     string
	trimmed   int
	window    int
	thinkMs   int64
	docType   string
	docTitle  string
	docStart  int // offset byte di content; -1 = tidak ada dokumen
}

// runAgent loop agen; dipanggil di goroutine sendiri.
func (a *App) runAgent(ctx context.Context, run *chatRun, u *store.User, conv *store.Conversation, p runParams) {
	defer run.cancel()
	st := &agentState{docStart: -1}
	started := time.Now()
	var firstReasoning time.Time
	errMsg := ""

	msgs, err := a.st.ListMessages(conv.ID)
	if err != nil {
		errMsg = "gagal memuat riwayat: " + err.Error()
	}
	history := historyFromMessages(msgs)
	if p.ForcedDoc != "" {
		history = append([]translate.Message{{Role: translate.RoleSystem,
			Content: []translate.Part{{Type: translate.PartText, Text: p.DocPrompts[p.ForcedDoc]}}}}, history...)
		st.docType, st.docStart = p.ForcedDoc, 0
	}
	tools := []translate.Tool{webSearchTool, webFetchTool}
	if p.DocTool && p.ForcedDoc == "" && len(p.DocPrompts) > 0 {
		tools = append(tools, createDocTool)
	}
	history = withChatSystemPrompt(history, a.chatSystemPrompt(u, tools, conv.SystemPrompt))

	text := func(s string) {
		if st.thinkMs == 0 && !firstReasoning.IsZero() {
			st.thinkMs = time.Since(firstReasoning).Milliseconds()
		}
		st.content.WriteString(s)
		run.emit(map[string]any{"t": "text", "v": s})
	}
	hooks := chatHooks{
		Text: text,
		Reasoning: func(s string) {
			if firstReasoning.IsZero() {
				firstReasoning = time.Now()
			}
			st.reasoning.WriteString(s)
			run.emit(map[string]any{"t": "reasoning", "v": s})
		},
		Model: func(m string) {
			st.model = m
			run.emit(map[string]any{"t": "model", "model": m})
		},
	}

	for round := 0; errMsg == "" && round <= maxAgentRounds; round++ {
		if c := st.content.String(); c != "" && !strings.HasSuffix(c, "\n") {
			text("\n\n")
		}
		roundStart := st.content.Len()
		if st.docStart >= 0 && p.ForcedDoc == "" {
			st.docStart = roundStart // dokumen dari tool = teks putaran terakhir
		}
		req := &translate.ChatRequest{Model: p.Model, Messages: history}
		if round < maxAgentRounds { // putaran terakhir tanpa tool: wajib menyimpulkan
			req.Tools = tools
		}
		res, err := a.chatOnce(ctx, u, req, hooks)
		if res != nil && res.Trimmed > st.trimmed {
			st.trimmed, st.window = res.Trimmed, res.Window
			run.emit(map[string]any{"t": "trimmed", "n": res.Trimmed, "window": res.Window})
		}
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				errMsg = agentErrorMessage(ctx, err)
			}
			break
		}
		if res.Finish != translate.FinishToolCalls || len(res.ToolCalls) == 0 {
			break
		}

		uses := make([]*toolUse, len(res.ToolCalls))
		calls := make([]translate.ToolCall, len(res.ToolCalls))
		for k, tc := range res.ToolCalls {
			t := &toolUse{ID: tc.ID, Name: tc.Name, Args: tc.Arguments, Round: round, Status: "run"}
			if t.ID == "" {
				t.ID = fmt.Sprintf("call_%d_%d", round, k)
			}
			var args struct{ URL, Query, Type string }
			_ = json.Unmarshal([]byte(tc.Arguments), &args)
			t.URL, t.Query, t.Doc = args.URL, args.Query, args.Type
			uses[k] = t
			calls[k] = translate.ToolCall{ID: t.ID, Name: t.Name, Arguments: orDefault(t.Args, "{}")}
			run.emit(map[string]any{"t": "tool", "tool": *t})
		}
		st.tools = append(st.tools, uses...)
		roundText := strings.TrimSpace(st.content.String()[roundStart:])
		am := translate.Message{Role: translate.RoleAssistant, ToolCalls: calls}
		if roundText != "" {
			am.Content = []translate.Part{{Type: translate.PartText, Text: roundText}}
		}
		history = append(history, am)

		var wg sync.WaitGroup
		for _, t := range uses {
			wg.Add(1)
			go func() {
				defer wg.Done()
				a.execTool(ctx, u, t, p)
				ev := map[string]any{"t": "tool_end", "id": t.ID, "status": t.Status}
				if t.Hits != nil {
					ev["hits"] = *t.Hits
				}
				if t.Status == "err" {
					ev["error"] = t.Result
				}
				run.emit(ev)
			}()
		}
		wg.Wait()
		for _, t := range uses {
			history = append(history, translate.Message{Role: translate.RoleTool, ToolCallID: t.ID,
				Content: []translate.Part{{Type: translate.PartText, Text: t.Result}}})
			if t.Name == "create_document" && t.Status == "ok" {
				st.docType, st.docStart = t.Doc, 0
				var args struct{ Title string }
				_ = json.Unmarshal([]byte(t.Args), &args)
				st.docTitle = args.Title
			}
		}
		if ctx.Err() != nil {
			break
		}
	}
	if st.thinkMs == 0 && !firstReasoning.IsZero() {
		st.thinkMs = time.Since(firstReasoning).Milliseconds()
	}
	stopped := errors.Is(ctx.Err(), context.Canceled)
	if errMsg != "" {
		text("\n\n**⚠ " + errMsg + "**")
	}
	a.finishRun(run, conv.ID, st, stopped, errMsg, time.Since(started))
}

// agentErrorMessage pesan error yang ramah untuk ditampilkan di jawaban.
func agentErrorMessage(ctx context.Context, err error) string {
	var qe *errQuota
	switch {
	case errors.As(err, &qe):
		return qe.reason
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "waktu proses habis (maks " + runMaxDuration.String() + ")"
	}
	return err.Error()
}

// finishRun menyimpan jawaban ke riwayat lalu mengirim event selesai.
func (a *App) finishRun(run *chatRun, convID int64, st *agentState, stopped bool, errMsg string, dur time.Duration) {
	raw := st.content.String()
	content := strings.TrimSpace(raw)
	meta := messageMeta{
		Reasoning: st.reasoning.String(), ThinkMs: st.thinkMs, Stopped: stopped,
		ContextTrimmed: st.trimmed, ContextWindow: st.window, Error: errMsg,
	}
	for _, t := range st.tools {
		if t.Status == "run" {
			t.Status, t.Result = "err", "ERROR: dibatalkan"
		}
		meta.Tools = append(meta.Tools, *t)
	}
	if st.docType != "" && st.docStart >= 0 && !stopped && errMsg == "" {
		lead := len(raw) - len(strings.TrimLeft(raw, " \t\r\n"))
		start := max(0, min(st.docStart-lead, len(content)))
		// offset dalam satuan UTF-16 agar cocok dengan String.slice di browser
		meta.Doc = &docInfo{Type: st.docType, Title: st.docTitle, Start: len(utf16.Encode([]rune(content[:start])))}
	}
	metaJSON, _ := json.Marshal(meta)
	done := map[string]any{"t": "done", "stopped": stopped, "ms": dur.Milliseconds()}
	if meta.Doc != nil {
		done["doc"] = meta.Doc
	}
	if content != "" || len(meta.Tools) > 0 || stopped || errMsg != "" {
		if m, err := a.st.AddMessageWithMeta(convID, "assistant", content, st.model, "", string(metaJSON)); err == nil {
			done["message_id"] = m.ID
		} else {
			done["save_error"] = err.Error()
		}
		a.st.TouchConversation(convID)
	}
	run.emit(done)
	run.finish()
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// execTool menjalankan satu tool; hasil (atau "ERROR: …") masuk ke t.Result.
func (a *App) execTool(ctx context.Context, u *store.User, t *toolUse, p runParams) {
	fail := func(msg string) { t.Status, t.Result = "err", "ERROR: "+msg }
	switch t.Name {
	case "web_search":
		if strings.TrimSpace(t.Query) == "" {
			fail("argumen query wajib")
			return
		}
		rs, err := a.search.Search(ctx, t.Query, 6)
		if err != nil {
			fail(a.searchError(err))
			return
		}
		n := len(rs)
		t.Hits = &n
		_ = a.st.Audit(u.ID, "tool.websearch", t.Query, nil, map[string]any{"backend": a.search.Name(), "results": n, "via": "agent"})
		if n == 0 {
			t.Status, t.Result = "ok", fmt.Sprintf("Tidak ada hasil untuk %q. Coba kueri lain yang lebih umum.", t.Query)
			return
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Hasil pencarian untuk %q:\n\n", t.Query)
		for i, r := range rs {
			fmt.Fprintf(&b, "%d. %s\n   URL: %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet)
		}
		b.WriteString("Baca halaman yang relevan dengan web_fetch bila cuplikan belum cukup.")
		t.Status, t.Result = "ok", b.String()
	case "web_fetch":
		if strings.TrimSpace(t.URL) == "" {
			fail("argumen url wajib")
			return
		}
		title, text, err := a.fetchPage(ctx, t.URL)
		if err != nil {
			fail(err.Error())
			return
		}
		_ = a.st.Audit(u.ID, "tool.webfetch", t.URL, nil, map[string]any{"chars": len([]rune(text)), "via": "agent"})
		t.Status, t.Result = "ok", "Judul: "+orDefault(title, "-")+"\nURL: "+t.URL+"\n\n"+text
	case "create_document":
		prompt, ok := p.DocPrompts[t.Doc]
		if !docTypes[t.Doc] || !ok {
			fail("type harus docx, pptx, xlsx, atau csv")
			return
		}
		t.Status, t.Result = "ok", prompt+"\n\nFile akan dirakit otomatis dari jawabanmu berikutnya. Tulis isi lengkap dokumen sekarang, tanpa memanggil create_document lagi."
	default:
		fail(fmt.Sprintf("tool %q tidak dikenal", t.Name))
	}
}

// ---- HTTP ----

// handleStartRun POST /api/me/conversations/{id}/runs — simpan pesan user
// lalu jalankan agen di latar belakang.
func (a *App) handleStartRun(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	conv, err := a.st.GetConversation(id)
	if err != nil || conv.UserID != ai.user.ID {
		writeJSON(w, 404, map[string]string{"error": "percakapan tidak ditemukan"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxChatBody+1))
	if err != nil || len(raw) > maxChatBody {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "permintaan terlalu besar"})
		return
	}
	var p runParams
	if err := json.Unmarshal(raw, &p); err != nil || p.Model == "" {
		writeJSON(w, 400, map[string]string{"error": "model wajib"})
		return
	}
	p.Content = strings.TrimSpace(p.Content)
	if p.Content == "" && len(p.Attachments) == 0 {
		writeJSON(w, 400, map[string]string{"error": "pesan kosong"})
		return
	}
	for k, v := range p.DocPrompts {
		if !docTypes[k] || len(v) > 32<<10 {
			delete(p.DocPrompts, k)
		}
	}
	if p.ForcedDoc != "" && p.DocPrompts[p.ForcedDoc] == "" {
		p.ForcedDoc = ""
	}
	userMeta := ""
	if len(p.Attachments) > 0 {
		b, _ := json.Marshal(messageMeta{Attachments: p.Attachments})
		if len(b) > maxMessageMeta {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "lampiran terlalu besar"})
			return
		}
		userMeta = string(b)
	}
	if qs := usage.QuotaCheck(a.st, ai.user.ID, 0); !qs.OK {
		writeJSON(w, 429, map[string]string{"error": qs.Reason})
		return
	}
	if _, err := a.chatRes.Resolve(p.Model); err != nil {
		writeJSON(w, 404, map[string]string{"error": err.Error()})
		return
	}
	run, ctx, ok := a.runs.start(conv.ID, ai.user.ID)
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "jawaban sebelumnya masih diproses"})
		return
	}
	um, err := a.st.AddMessageWithMeta(conv.ID, "user", p.Content, "", "", userMeta)
	if err != nil {
		run.cancel()
		run.finish()
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.st.TouchConversation(conv.ID)
	go a.runAgent(ctx, run, ai.user, conv, p)
	writeJSON(w, http.StatusCreated, map[string]any{"run_id": run.ID, "user_message_id": um.ID})
}

func (a *App) ownRun(w http.ResponseWriter, r *http.Request) *chatRun {
	run := a.runs.get(r.PathValue("rid"))
	if run == nil || run.UserID != authFrom(r).user.ID {
		writeJSON(w, 404, map[string]string{"error": "proses tidak ditemukan atau sudah kedaluwarsa"})
		return nil
	}
	return run
}

// handleRunEvents GET /api/me/runs/{rid}/events?from=N — SSE: putar ulang
// event sejak N lalu ikuti secara langsung sampai run selesai.
func (a *App) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	run := a.ownRun(w, r)
	if run == nil {
		return
	}
	from := 0
	fmt.Sscan(r.URL.Query().Get("from"), &from)
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	fmt.Fprint(w, ": ok\n\n")
	flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		evs, done, wait := run.snapshot(from)
		for _, ev := range evs {
			fmt.Fprintf(w, "data: %s\n\n", ev)
		}
		from += len(evs)
		if done {
			fmt.Fprint(w, "data: [END]\n\n")
			flush()
			return
		}
		flush()
		select {
		case <-wait:
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flush()
		case <-r.Context().Done():
			return // klien pergi; run tetap berjalan
		}
	}
}

// handleCancelRun POST /api/me/runs/{rid}/cancel.
func (a *App) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	run := a.ownRun(w, r)
	if run == nil {
		return
	}
	run.cancel()
	writeJSON(w, 200, map[string]string{"ok": "true"})
}
