package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jenderal/jenderalrouter/internal/apigate"
	"github.com/jenderal/jenderalrouter/internal/router"
	"github.com/jenderal/jenderalrouter/internal/translate"
	"github.com/jenderal/jenderalrouter/internal/usage"
)

// chatInference menangani POST /v1/chat/completions (format OpenAI).
func (a *App) chatInference(w http.ResponseWriter, r *http.Request) {
	a.infer(w, r, translate.FormatOpenAI)
}

// messagesInference menangani POST /v1/messages (format Anthropic, untuk
// Claude Code via ANTHROPIC_BASE_URL — FR-3.2).
func (a *App) messagesInference(w http.ResponseWriter, r *http.Request) {
	a.infer(w, r, translate.FormatAnthropic)
}

// infer jalur inferensi bersama: auth → kuota → resolusi → runner → render.
func (a *App) infer(w http.ResponseWriter, r *http.Request, inbound translate.Format) {
	start := time.Now()
	ac, apiErr := a.gate.Authenticate(r)
	if apiErr != nil {
		a.writeGateError(w, inbound, apiErr)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		a.writeGateError(w, inbound, &apigate.APIError{Status: 400, Code: "invalid_request", Message: "baca body gagal"})
		return
	}
	var internal *translate.ChatRequest
	if inbound == translate.FormatAnthropic {
		internal, err = translate.ParseAnthropicRequest(body)
	} else {
		internal, err = translate.ParseOpenAIRequest(body)
	}
	if err != nil {
		a.writeGateError(w, inbound, &apigate.APIError{Status: 400, Code: "invalid_request", Message: err.Error()})
		return
	}
	if internal.Model == "" {
		a.writeGateError(w, inbound, &apigate.APIError{Status: 400, Code: "invalid_request", Message: "field model wajib"})
		return
	}
	// model/combo harus diizinkan key (FR-4.3) → 403 model_not_allowed
	// (nama request dicocokkan juga dengan alias ↔ public_id-nya)
	if !modelAllowedForRequest(ac, a.st, internal.Model) {
		a.writeGateError(w, inbound, apigate.ErrModelNotAllowed(internal.Model))
		return
	}
	// kuota (FR-4.4) → 429 quota_exceeded
	if qs := usage.QuotaCheck(a.st, ac.User.ID, ac.Key.ID); !qs.OK {
		e := &apigate.APIError{Status: 429, Code: "quota_exceeded", Message: qs.Reason, RetryAfter: qs.RetryAfter}
		a.writeGateError(w, inbound, e)
		return
	}
	// RPM (FR-4.4) → 429 rate_limited
	if apiErr := a.gate.CheckRateLimit(ac.Key.ID, ac.Key.RPM); apiErr != nil {
		a.writeGateError(w, inbound, apiErr)
		return
	}
	// resolusi model/alias/combo → 404 model_not_found
	steps, err := a.resolver.Resolve(internal.Model)
	if err != nil {
		a.writeGateError(w, inbound, apigate.ErrModelNotFound(internal.Model))
		return
	}

	reqTokens := translate.EstimateMessages(internal.Messages)
	rec := usage.Entry{
		TS: start, UserID: ac.User.ID, KeyID: ac.Key.ID,
		RequestedModel: internal.Model, Status: http.StatusOK, Stream: internal.Stream,
		IP: apigate.ClientIP(r), TokensIn: reqTokens,
		QuotaTargets: []usage.QuotaTarget{{Scope: "user", ScopeID: ac.User.ID}, {Scope: "key", ScopeID: ac.Key.ID}},
	}

	// SSE header + keep-alive selama menunggu token pertama (FR-6.4)
	keepDone := make(chan struct{})
	if internal.Stream {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		if _, canFlush := w.(http.Flusher); canFlush {
			go func() {
				t := time.NewTicker(3 * time.Second)
				defer t.Stop()
				for {
					select {
					case <-keepDone:
						return
					case <-t.C:
						fmt.Fprint(w, ": keep-alive\n\n")
					}
				}
			}()
		}
	}

	exec := newStepExecutor(a, internal, internal.Stream)
	runner := &router.Runner{Breakers: a.breakers, Sender: exec.send, MaxRetrySameProvider: 1}
	res, attempts, runErr := runner.Run(r.Context(), steps)
	close(keepDone) // hentikan keep-alive sebelum menulis body

	rec.Attempts = len(attempts) + 1
	ttftMs := time.Since(start).Milliseconds()

	if runErr != nil {
		rec.Status = statusForError(runErr)
		rec.ErrorCode = codeForError(runErr)
		rec.LatencyMs = time.Since(start).Milliseconds()
		a.rec.Record(rec)
		metrics.observeRequest("", rec.Status, time.Since(start).Seconds(), rec.TokensIn, rec.TokensOut)
		for _, at := range attempts {
			code := "error"
			if ue, ok := at.Err.(*translate.UpstreamError); ok && ue.Code != "" {
				code = ue.Code
			}
			metrics.observeUpstreamError(at.ModelPublic, code)
		}
		a.writeUpstreamError(w, inbound, internal.Stream, runErr, attempts)
		return
	}

	// header rute aktual (FR-2.6) — nama tampilan (alias) yang dikonsumsi klien
	w.Header().Set("X-Route-Provider", res.ProviderName)
	w.Header().Set("X-Route-Model", res.ModelDisplay)
	w.Header().Set("X-Route-Attempts", fmt.Sprintf("%d", rec.Attempts))

	rec.ProviderID = res.ProviderID
	rec.ProviderName = res.ProviderName
	rec.ModelID = steps[res.StepIndex].ModelID
	rec.ModelName = res.ModelDisplay

	if internal.Stream {
		// FR-2.3: fallback sudah selesai di runner; mulai sekarang error diteruskan
		a.streamToClient(w, inbound, res, &rec, start, ttftMs, reqTokens)
		return
	}

	resp := res.Response
	resp.Model = res.ModelDisplay // echo nama tampilan (alias), bukan nama upstream
	tokensIn, tokensOut := resp.Usage.PromptTokens, resp.Usage.CompletionTokens
	if tokensIn == 0 && tokensOut == 0 {
		tokensIn, tokensOut = reqTokens, translate.EstimateTokens(resp.Content)
		rec.TokensEstimated = true
	}
	resp.Usage = translate.Usage{PromptTokens: tokensIn, CompletionTokens: tokensOut, Estimated: rec.TokensEstimated}
	rec.TokensIn, rec.TokensOut = tokensIn, tokensOut
	rec.LatencyMs = time.Since(start).Milliseconds()
	rec.TTFTMs = rec.LatencyMs
	a.applyCost(&rec, res.ModelPublic)
	a.rec.Record(rec)
	metrics.observeRequest(res.ProviderName, http.StatusOK, time.Since(start).Seconds(), tokensIn, tokensOut)
	_ = a.gate.RecordTPM(ac.Key.ID, ac.Key.TPM, tokensIn+tokensOut)

	var out []byte
	if inbound == translate.FormatAnthropic {
		out, err = translate.RenderAnthropicResponse(resp)
	} else {
		out, err = translate.RenderOpenAIResponse(resp)
	}
	if err != nil {
		a.writeUpstreamError(w, inbound, false, err, attempts)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

// streamToClient meneruskan event upstream ke klien dalam format endpoint.
func (a *App) streamToClient(w http.ResponseWriter, inbound translate.Format, res *router.AttemptResult, rec *usage.Entry, start time.Time, ttftMs int64, reqTokens int) {
	flusher, _ := w.(http.Flusher)
	w.WriteHeader(http.StatusOK)
	if flusher != nil {
		flusher.Flush()
	}
	defer func() {
		rec.Status = http.StatusOK
		rec.LatencyMs = time.Since(start).Milliseconds()
		if rec.TTFTMs == 0 {
			rec.TTFTMs = ttftMs
		}
		a.applyCost(rec, res.ModelPublic)
		a.rec.Record(*rec)
		metrics.observeRequest(res.ProviderName, http.StatusOK, time.Since(start).Seconds(), rec.TokensIn, rec.TokensOut)
	}()
	if inbound == translate.FormatAnthropic {
		a.streamAnthropicOut(w, flusher, res, rec, reqTokens)
		return
	}
	a.streamOpenAIOut(w, flusher, res, rec, reqTokens)
}

func (a *App) streamOpenAIOut(w http.ResponseWriter, flusher http.Flusher, res *router.AttemptResult, rec *usage.Entry, reqTokens int) {
	id := genChunkID()
	model := res.ModelDisplay // echo nama tampilan (alias)
	var content strings.Builder
	first := true
	tokensIn, tokensOut := 0, 0

	emit := func(payload string) {
		if payload == "" {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
	}
	begin := func() {
		if first {
			emit(translate.RenderOpenAISSEChunk(translate.Event{Type: translate.EventMessageBegin}, id, model))
			first = false
		}
	}

	for {
		e, ok := res.Stream.Next()
		if !ok {
			break
		}
		switch e.Type {
		case translate.EventDelta:
			begin()
			if e.Delta.Text != "" {
				content.WriteString(e.Delta.Text)
			}
			emit(translate.RenderOpenAISSEChunk(e, id, model))
		case translate.EventUsage:
			if e.Usage != nil {
				tokensIn, tokensOut = e.Usage.PromptTokens, e.Usage.CompletionTokens
			}
		case translate.EventMessageEnd:
			begin()
			reason := translate.FinishStop
			if e.Delta != nil && e.Delta.FinishReason != "" {
				reason = e.Delta.FinishReason
			}
			emit(translate.RenderOpenAISSEChunk(translate.Event{
				Type: translate.EventMessageEnd, Delta: &translate.StreamDelta{FinishReason: reason},
			}, id, model))
		case translate.EventError:
			// FR-2.3: error setelah token pertama diteruskan, bukan fallback
			emit(string(translate.RenderOpenAIError(http.StatusBadGateway, "upstream_error", e.Error.Message)))
		}
	}
	if first {
		// stream kosong: kirim begin+end agar klien tidak menggantung
		begin()
		emit(translate.RenderOpenAISSEChunk(translate.Event{
			Type: translate.EventMessageEnd, Delta: &translate.StreamDelta{FinishReason: translate.FinishStop},
		}, id, model))
	}
	if tokensIn == 0 && tokensOut == 0 {
		tokensIn = reqTokens
		tokensOut = translate.EstimateTokens(content.String())
		rec.TokensEstimated = true
	}
	rec.TokensIn, rec.TokensOut = tokensIn, tokensOut
	// chunk usage gaya stream_options.include_usage (choices kosong)
	emit(translate.RenderOpenAISSEChunk(translate.Event{
		Type:  translate.EventUsage,
		Usage: &translate.Usage{PromptTokens: tokensIn, CompletionTokens: tokensOut},
	}, id, model))
	emit("[DONE]")
	res.Stream.Close()
}

func (a *App) streamAnthropicOut(w http.ResponseWriter, flusher http.Flusher, res *router.AttemptResult, rec *usage.Entry, reqTokens int) {
	writer := translate.NewAnthropicSSEWriter()
	var content strings.Builder
	first := true
	tokensIn, tokensOut := 0, 0

	writeFrames := func(frames []string) {
		for _, f := range frames {
			fmt.Fprint(w, f)
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	begin := func(id, model string) {
		if first {
			fmt.Fprint(w, writer.Begin(id, model, 0))
			if flusher != nil {
				flusher.Flush()
			}
			first = false
		}
	}

	for {
		e, ok := res.Stream.Next()
		if !ok {
			break
		}
		switch e.Type {
		case translate.EventMessageBegin:
			begin(e.ID, res.ModelDisplay) // alias menang atas nama upstream
		case translate.EventDelta:
			begin("", res.ModelDisplay)
			if e.Delta.Text != "" {
				content.WriteString(e.Delta.Text)
			}
			writeFrames(writer.WriteEvent(e))
		case translate.EventUsage:
			if e.Usage != nil {
				tokensIn, tokensOut = e.Usage.PromptTokens, e.Usage.CompletionTokens
			}
			writeFrames(writer.WriteEvent(e))
		case translate.EventMessageEnd:
			begin("", res.ModelDisplay)
			writeFrames(writer.WriteEvent(e))
		case translate.EventError:
			// FR-2.3: error di tengah stream diteruskan sebagai event error
			fmt.Fprint(w, frameAnthropicError(e.Error))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
	if first {
		begin("", res.ModelDisplay)
	}
	fmt.Fprint(w, writer.Finish())
	if flusher != nil {
		flusher.Flush()
	}
	if tokensIn == 0 && tokensOut == 0 {
		tokensIn = reqTokens
		tokensOut = translate.EstimateTokens(content.String())
		rec.TokensEstimated = true
	}
	rec.TokensIn, rec.TokensOut = tokensIn, tokensOut
	res.Stream.Close()
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// applyCost menghitung estimasi biaya dari harga model (FR-1.8, FR-6.6).
func (a *App) applyCost(rec *usage.Entry, publicID string) {
	if m, err := a.st.FindModelByPublicOrAlias(publicID); err == nil {
		rec.CostUSD = usage.CostUSDCalculate(rec.TokensIn, rec.TokensOut, m.PriceInPer1M, m.PriceOutPer1M)
	}
}

// writeGateError menulis error auth/kuota/ratelimit sesuai format endpoint (FR-3.4).
func (a *App) writeGateError(w http.ResponseWriter, inbound translate.Format, e *apigate.APIError) {
	if e.RetryAfter > 0 {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", e.RetryAfter))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(e.Status)
	if inbound == translate.FormatAnthropic {
		_, _ = w.Write(translate.RenderAnthropicError(translate.MapStatusToAnthropicErrorType(e.Status), e.Message))
		return
	}
	_, _ = w.Write(translate.RenderOpenAIError(e.Status, e.Code, e.Message))
}

// writeUpstreamError menulis kegagalan semua langkah combo (502/504, §9)
// dengan rincian tiap percobaan di error.attempts.
func (a *App) writeUpstreamError(w http.ResponseWriter, inbound translate.Format, streaming bool, err error, attempts []router.AttemptData) {
	status := statusForError(err)
	code := codeForError(err)
	msg := err.Error()

	type attemptInfo struct {
		Model string `json:"model"`
		Error string `json:"error"`
	}
	infos := make([]attemptInfo, 0, len(attempts))
	for _, at := range attempts {
		infos = append(infos, attemptInfo{Model: at.ModelPublic, Error: at.Err.Error()})
	}

	if streaming {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.WriteHeader(status)
		flusher, _ := w.(http.Flusher)
		if inbound == translate.FormatAnthropic {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", jsonCompact(map[string]any{
				"type":  "error",
				"error": map[string]any{"type": "api_error", "message": msg, "attempts": infos},
			}))
		} else {
			fmt.Fprintf(w, "data: %s\n\n", jsonCompact(map[string]any{
				"error": map[string]any{"message": msg, "type": "upstream_error", "code": code, "attempts": infos},
			}))
			fmt.Fprint(w, "data: [DONE]\n\n")
		}
		if flusher != nil {
			flusher.Flush()
		}
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if inbound == translate.FormatAnthropic {
		_, _ = w.Write(translate.RenderAnthropicError(translate.MapStatusToAnthropicErrorType(status), msg))
		return
	}
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{"message": msg, "type": "upstream_error", "code": code, "attempts": infos},
	})
	_, _ = w.Write(body)
}

func statusForError(err error) int {
	if ue, ok := err.(*translate.UpstreamError); ok {
		switch {
		case ue.StatusCode == 0:
			return http.StatusBadGateway
		case ue.StatusCode == 408:
			return http.StatusGatewayTimeout
		case ue.StatusCode >= 500:
			return http.StatusBadGateway
		default:
			// 4xx non-retryable (mis. 400 request salah) diteruskan apa adanya
			return ue.StatusCode
		}
	}
	return http.StatusBadGateway
}

func codeForError(err error) string {
	if ue, ok := err.(*translate.UpstreamError); ok {
		if ue.Code != "" {
			return ue.Code
		}
		switch {
		case ue.StatusCode == 408:
			return "upstream_timeout"
		case ue.StatusCode == 0 || ue.StatusCode >= 500:
			return "upstream_error"
		}
		return "provider_error"
	}
	return "upstream_error"
}

func jsonCompact(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func genChunkID() string {
	return fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
}

func frameAnthropicError(ue *translate.UpstreamError) string {
	return "event: error\ndata: " + jsonCompact(map[string]any{
		"type":  "error",
		"error": map[string]any{"type": "api_error", "message": ue.Message},
	}) + "\n\n"
}
