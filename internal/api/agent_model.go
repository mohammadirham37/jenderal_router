package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jenderal/jenderalrouter/internal/router"
	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/translate"
	"github.com/jenderal/jenderalrouter/internal/usage"
)

// chatHooks callback streaming satu panggilan model (dipanggil berurutan
// dari goroutine pemanggil chatOnce).
type chatHooks struct {
	Text      func(string)
	Reasoning func(string)
	Model     func(string)
}

// chatOnceResult hasil satu panggilan model.
type chatOnceResult struct {
	Content   string
	Reasoning string
	ToolCalls []translate.ToolCall
	Finish    string
	Trimmed   int // pesan lama yang dipangkas demi context window
	Window    int
}

// errQuota kuota user habis (pesan aman ditampilkan).
type errQuota struct{ reason string }

func (e *errQuota) Error() string { return e.reason }

// chatOnce menjalankan satu panggilan model streaming lewat pipeline
// kanal chat (resolve rute + fallback, pemangkasan konteks, pencatatan
// pemakaian & kuota). Dipakai agen chat server-side; endpoint
// /api/me/chat (format OpenAI) tetap punya jalurnya sendiri.
func (a *App) chatOnce(ctx context.Context, u *store.User, req *translate.ChatRequest, h chatHooks) (*chatOnceResult, error) {
	if qs := usage.QuotaCheck(a.st, u.ID, 0); !qs.OK {
		return nil, &errQuota{qs.Reason}
	}
	steps, err := a.chatRes.Resolve(req.Model)
	if err != nil {
		return nil, err
	}
	out := &chatOnceResult{}
	budget, window := a.chatContextBudget(steps)
	req.Messages, out.Trimmed = trimForContext(req.Messages, budget)
	out.Window = window
	req.Stream = true

	start := time.Now()
	reqTokens := translate.EstimateMessages(req.Messages)
	rec := usage.Entry{
		TS: start, UserID: u.ID, RequestedModel: req.Model,
		Status: http.StatusOK, Stream: true, TokensIn: reqTokens,
		QuotaTargets: []usage.QuotaTarget{{Scope: "user", ScopeID: u.ID}},
	}
	exec := newStepExecutor(a, req, true)
	runner := &router.Runner{Breakers: a.breakers, Sender: exec.send, MaxRetrySameProvider: 1}
	res, attempts, runErr := runner.Run(ctx, steps)
	rec.Attempts = len(attempts) + 1
	if runErr != nil {
		rec.Status = statusForError(runErr)
		rec.LatencyMs = time.Since(start).Milliseconds()
		a.rec.Record(rec)
		return nil, runErr
	}
	defer res.Stream.Close()

	display := res.ModelDisplay
	if display == "" {
		display = res.ModelPublic
	}
	rec.ProviderID, rec.ProviderName = res.ProviderID, res.ProviderName
	rec.ModelID, rec.ModelName = steps[res.StepIndex].ModelID, display
	if h.Model != nil {
		h.Model(display)
	}

	var content, reasoning strings.Builder
	calls := map[int]*translate.ToolCall{}
	tokensIn, tokensOut := 0, 0
	for {
		e, ok := res.Stream.Next()
		if !ok {
			break
		}
		switch e.Type {
		case translate.EventDelta:
			if e.Delta == nil {
				continue
			}
			if e.Delta.Reasoning != "" {
				reasoning.WriteString(e.Delta.Reasoning)
				if h.Reasoning != nil {
					h.Reasoning(e.Delta.Reasoning)
				}
			}
			for _, tc := range e.Delta.ToolCalls {
				c := calls[tc.Index]
				if c == nil {
					c = &translate.ToolCall{}
					calls[tc.Index] = c
				}
				if tc.ID != "" {
					c.ID = tc.ID
				}
				c.Name += tc.Name
				c.Arguments += tc.Arguments
			}
			if e.Delta.Text != "" {
				content.WriteString(e.Delta.Text)
				if h.Text != nil {
					h.Text(e.Delta.Text)
				}
			}
			if e.Delta.FinishReason != "" {
				out.Finish = e.Delta.FinishReason
			}
		case translate.EventMessageEnd:
			if e.Delta != nil && e.Delta.FinishReason != "" {
				out.Finish = e.Delta.FinishReason
			}
		case translate.EventUsage:
			if e.Usage != nil {
				tokensIn, tokensOut = e.Usage.PromptTokens, e.Usage.CompletionTokens
			}
		case translate.EventError:
			if e.Error != nil {
				runErr = e.Error
			}
		}
	}
	if tokensIn == 0 && tokensOut == 0 {
		tokensIn, tokensOut = reqTokens, translate.EstimateTokens(content.String())
		rec.TokensEstimated = true
	}
	rec.TokensIn, rec.TokensOut = tokensIn, tokensOut
	rec.LatencyMs = time.Since(start).Milliseconds()
	if ctx.Err() != nil && runErr == nil {
		runErr = ctx.Err()
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		rec.Status = statusForError(runErr)
	}
	a.applyCost(&rec, res.ModelPublic)
	a.rec.Record(rec)

	idx := make([]int, 0, len(calls))
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		if c := calls[i]; c.Name != "" {
			out.ToolCalls = append(out.ToolCalls, *c)
		}
	}
	out.Content, out.Reasoning = content.String(), reasoning.String()
	return out, runErr
}
