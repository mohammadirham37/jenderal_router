package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jenderal/jenderalrouter/internal/router"
	"github.com/jenderal/jenderalrouter/internal/translate"
	"github.com/jenderal/jenderalrouter/internal/usage"
)

// Endpoint /api/me/* — khusus pengguna login dashboard (playground, F-12).
// Inferensi lewat sesi, tanpa API key; kuota dihitung pada scope user.

// handleMeModels daftar model+combo aktif untuk dropdown playground.
func (a *App) handleMeModels(w http.ResponseWriter, r *http.Request) {
	models, err := a.st.ListEnabledModels()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	type entry struct {
		ID    string `json:"id"`
		Kind  string `json:"kind"` // model|combo
		Owner string `json:"owner"`
		Local bool   `json:"local"`
	}
	entries := []entry{}
	for _, m := range models {
		entries = append(entries, entry{ID: m.PublicID, Kind: "model", Owner: m.ProviderName, Local: m.ProviderType == "llamastash"})
	}
	if combos, err := a.st.ListCombos(); err == nil {
		for _, c := range combos {
			entries = append(entries, entry{ID: c.Name, Kind: "combo", Owner: "combo"})
		}
	}
	writeJSON(w, 200, map[string]any{"models": entries})
}

// handleMeUsage sisa kuota user yang sedang login (FR-7.5).
func (a *App) handleMeUsage(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	type quotaView struct {
		Period     string  `json:"period"`
		TokenLimit int64   `json:"token_limit"`
		UsedTokens int64   `json:"used_tokens"`
		CostLimit  float64 `json:"cost_limit_usd"`
		UsedCost   float64 `json:"used_cost_usd"`
		ResetAt    string  `json:"reset_at"`
	}
	quotas := []quotaView{}
	for _, q := range collectUserKeyQuotas(a.st, ai.user.ID, 0) {
		quotas = append(quotas, quotaView{Period: q.Period, TokenLimit: q.TokenLimit,
			UsedTokens: q.UsedTokens, CostLimit: q.CostLimitUSD, UsedCost: q.UsedCost, ResetAt: q.ResetAt})
	}
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	res, err := usage.GetAnalytics(a.st, from, now.Add(time.Minute))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	var mine usage.UserUsage
	for _, u := range res.PerUser {
		if u.UserID == ai.user.ID {
			mine = u
			break
		}
	}
	writeJSON(w, 200, map[string]any{
		"quotas": quotas,
		"today":  map[string]any{"requests": mine.Requests, "tokens_in": mine.TokensIn, "tokens_out": mine.TokensOut},
	})
}

// handleSaveMessage menyimpan pesan riwayat percakapan (FR-7.2).
func (a *App) handleSaveMessage(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, err := a.st.GetConversation(id)
	if err != nil || c.UserID != ai.user.ID {
		writeJSON(w, 404, map[string]string{"error": "percakapan tidak ditemukan"})
		return
	}
	var req struct {
		Role    string `json:"role"`
		Content string `json:"content"`
		Model   string `json:"model"`
	}
	if err := decodeBody(w, r, &req); err != nil || req.Content == "" {
		writeJSON(w, 400, map[string]string{"error": "content wajib"})
		return
	}
	if req.Role != "user" && req.Role != "assistant" && req.Role != "system" {
		req.Role = "user"
	}
	if _, err := a.st.AddMessage(id, req.Role, req.Content, req.Model, ""); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.st.TouchConversation(id)
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

// handleMeChat proxy chat playground: auth sesi → pipeline inferensi yang
// sama dengan /v1, stream SSE format OpenAI (dikonsumsi api.js).
func (a *App) handleMeChat(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "baca body gagal"})
		return
	}
	// body berformat OpenAI chat (content string/array) → parser toleran
	internal, err := translate.ParseOpenAIRequest(raw)
	if err != nil || internal.Model == "" || len(internal.Messages) == 0 {
		writeJSON(w, 400, map[string]string{"error": "model dan messages wajib"})
		return
	}
	var opt struct {
		Stream *bool `json:"stream"`
	}
	_ = json.Unmarshal(raw, &opt)
	internal.Stream = opt.Stream == nil || *opt.Stream
	// kuota user (FR-4.4)
	if qs := usage.QuotaCheck(a.st, ai.user.ID, 0); !qs.OK {
		writeJSON(w, 429, map[string]string{"error": qs.Reason})
		return
	}
	steps, err := a.resolver.Resolve(internal.Model)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "model tidak ditemukan: " + internal.Model})
		return
	}

	start := time.Now()
	reqTokens := translate.EstimateMessages(internal.Messages)
	rec := usage.Entry{
		TS: start, UserID: ai.user.ID, RequestedModel: internal.Model,
		Status: http.StatusOK, Stream: internal.Stream, TokensIn: reqTokens,
		QuotaTargets: []usage.QuotaTarget{{Scope: "user", ScopeID: ai.user.ID}},
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	// keep-alive untuk proksi berbatas idle (Cloudflare Tunnel/nginx):
	// komentar SSE berkala + tulisan instan agar byte pertama < 100 dtk.
	var wmu sync.Mutex
	fmt.Fprint(w, ": ok\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	ka := startSSEKeepAlive(w, &wmu, 15*time.Second)
	defer ka.Stop()

	exec := newStepExecutor(a, internal, internal.Stream)
	runner := &router.Runner{Breakers: a.breakers, Sender: exec.send, MaxRetrySameProvider: 1}
	res, attempts, runErr := runner.Run(r.Context(), steps)
	rec.Attempts = len(attempts) + 1

	fail := func(status int, msg string) {
		rec.Status = status
		rec.LatencyMs = time.Since(start).Milliseconds()
		a.rec.Record(rec)
		wmu.Lock()
		fmt.Fprintf(w, "data: %s\n\n", jsonCompact(map[string]any{
			"error": map[string]any{"message": msg, "attempts": len(attempts)},
		}))
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		wmu.Unlock()
	}
	if runErr != nil {
		fail(statusForError(runErr), runErr.Error())
		return
	}

	rec.ProviderID = res.ProviderID
	rec.ProviderName = res.ProviderName
	rec.ModelID = steps[res.StepIndex].ModelID
	rec.ModelName = res.ModelPublic
	w.Header().Set("X-Route-Provider", res.ProviderName)
	w.Header().Set("X-Route-Model", res.ModelPublic)

	id := genChunkID()
	var content strings.Builder
	var reasoning strings.Builder
	servedModel := ""
	tokensIn, tokensOut := 0, 0
	if internal.Stream {
		first := true
		for {
			e, ok := res.Stream.Next()
			if !ok {
				break
			}
			if e.Model != "" {
				servedModel = e.Model
			}
			switch e.Type {
			case translate.EventDelta:
				wmu.Lock()
				if first {
					fmt.Fprintf(w, "data: %s\n\n", renderMeChunk(id, res.ModelPublic, map[string]any{"role": "assistant", "content": ""}))
					first = false
				}
				if e.Delta.Reasoning != "" {
					reasoning.WriteString(e.Delta.Reasoning)
					fmt.Fprintf(w, "data: %s\n\n", renderMeChunk(id, res.ModelPublic, map[string]any{"reasoning_content": e.Delta.Reasoning}))
				}
				if e.Delta.Text != "" {
					content.WriteString(e.Delta.Text)
					fmt.Fprintf(w, "data: %s\n\n", renderMeChunk(id, res.ModelPublic, map[string]any{"content": e.Delta.Text}))
				}
				wmu.Unlock()
			case translate.EventUsage:
				if e.Usage != nil {
					tokensIn, tokensOut = e.Usage.PromptTokens, e.Usage.CompletionTokens
				}
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if tokensIn == 0 && tokensOut == 0 {
			tokensIn = reqTokens
			tokensOut = translate.EstimateTokens(content.String())
			rec.TokensEstimated = true
		}
		rec.TokensIn, rec.TokensOut = tokensIn, tokensOut
		meta := map[string]any{
			"id": id, "choices": []any{}, "usage": map[string]any{
				"prompt_tokens": tokensIn, "completion_tokens": tokensOut, "total_tokens": tokensIn + tokensOut,
			},
		}
		if servedModel != "" {
			meta["served_model"] = servedModel
		}
		if reasoning.Len() > 0 {
			meta["reasoning_content"] = reasoning.String()
		}
		wmu.Lock()
		fmt.Fprintf(w, "data: %s\n\n", jsonCompact(meta))
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		wmu.Unlock()
	} else {
		resp := res.Response
		content.WriteString(resp.Content)
		tokensIn, tokensOut = resp.Usage.PromptTokens, resp.Usage.CompletionTokens
		if tokensIn == 0 && tokensOut == 0 {
			tokensIn = reqTokens
			tokensOut = translate.EstimateTokens(resp.Content)
			rec.TokensEstimated = true
		}
		rec.TokensIn, rec.TokensOut = tokensIn, tokensOut
		chunk := map[string]any{"content": resp.Content}
		if resp.Reasoning != "" {
			chunk["reasoning_content"] = resp.Reasoning
		}
		if servedModel != "" {
			chunk["served_model"] = servedModel
		}
		wmu.Lock()
		fmt.Fprintf(w, "data: %s\n\n", renderMeChunk(id, res.ModelPublic, chunk))
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		wmu.Unlock()
	}
	res.Stream.Close()
	rec.Status = http.StatusOK
	rec.LatencyMs = time.Since(start).Milliseconds()
	a.applyCost(&rec, res.ModelPublic)
	a.rec.Record(rec)
}

// renderMeChunk chunk SSE gaya OpenAI untuk playground.
func renderMeChunk(id, model string, delta map[string]any) string {
	b, _ := json.Marshal(map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model,
		"choices": []map[string]any{{"index": 0, "delta": delta}},
	})
	return string(b)
}

var _ = io.EOF
