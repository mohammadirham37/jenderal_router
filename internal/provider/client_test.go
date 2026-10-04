package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jenderal/jenderalrouter/internal/translate"
)

func openaiReq(model string, stream bool) *translate.ChatRequest {
	return &translate.ChatRequest{
		Model: model,
		Messages: []translate.Message{
			{Role: translate.RoleUser, Content: []translate.Part{{Type: translate.PartText, Text: "halo"}}},
		},
		Stream: stream,
	}
}

func TestCompleteOpenAIMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer sk-mock" {
			t.Errorf("auth = %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"cmpl-1","model":"mock-x","choices":[{"index":0,"message":{"role":"assistant","content":"Halo juga"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
	}))
	defer srv.Close()

	c := NewClient()
	resp, err := c.Complete(context.Background(), UpstreamReq{
		Format: translate.FormatOpenAI, BaseURL: srv.URL, APIKey: "sk-mock",
		Internal: openaiReq("mock-x", false),
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "Halo juga" || resp.Usage.PromptTokens != 3 {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestStreamOpenAIMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for _, data := range []string{
			`{"id":"c1","model":"m","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			`{"id":"c1","model":"m","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}`,
			`{"id":"c1","model":"m","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}`,
			`{"id":"c1","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`{"id":"c1","model":"m","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":2}}`,
			`[DONE]`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", data)
			f.Flush()
		}
	}))
	defer srv.Close()

	c := NewClient()
	es, err := c.Stream(context.Background(), UpstreamReq{
		Format: translate.FormatOpenAI, BaseURL: srv.URL, APIKey: "k",
		Internal: openaiReq("m", true), Stream: true,
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer es.Close()
	var text strings.Builder
	var sawEnd, sawUsage bool
	for {
		e, ok := es.Next()
		if !ok {
			break
		}
		switch e.Type {
		case translate.EventDelta:
			text.WriteString(e.Delta.Text)
		case translate.EventMessageEnd:
			sawEnd = true
		case translate.EventUsage:
			sawUsage = true
		}
	}
	if text.String() != "Hi!" || !sawEnd || !sawUsage {
		t.Fatalf("text=%q end=%v usage=%v", text.String(), sawEnd, sawUsage)
	}
	if es.Usage.PromptTokens != 4 || es.Usage.CompletionTokens != 2 {
		t.Errorf("stream usage = %+v", es.Usage)
	}
}

func TestStreamAnthropicMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "an-key" || r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("headers salah")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for _, frame := range []string{
			"message_start", `{"type":"message_start","message":{"id":"msg_1","model":"m","usage":{"input_tokens":7,"output_tokens":0}}}`,
			"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Ok"}}`,
			"content_block_stop", `{"type":"content_block_stop","index":0}`,
			"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
			"message_stop", `{"type":"message_stop"}`,
		} {
			if len(frame) < 30 && !strings.HasPrefix(frame, "{") {
				fmt.Fprintf(w, "event: %s\n", frame)
			} else {
				fmt.Fprintf(w, "data: %s\n\n", frame)
			}
			f.Flush()
		}
	}))
	defer srv.Close()

	c := NewClient()
	es, err := c.Stream(context.Background(), UpstreamReq{
		Format: translate.FormatAnthropic, BaseURL: srv.URL, APIKey: "an-key",
		Internal: openaiReq("claude-x", true), Stream: true,
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer es.Close()
	var text strings.Builder
	for {
		e, ok := es.Next()
		if !ok {
			break
		}
		if e.Type == translate.EventDelta {
			text.WriteString(e.Delta.Text)
		}
	}
	if text.String() != "Ok" {
		t.Errorf("text = %q", text.String())
	}
	if es.Usage.PromptTokens != 7 || es.Usage.CompletionTokens != 3 {
		t.Errorf("usage = %+v", es.Usage)
	}
}

func TestStreamGeminiMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/models/mock-gem:streamGenerateContent") {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "g-key" {
			t.Error("api key header salah")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for _, data := range []string{
			`{"candidates":[{"content":{"parts":[{"text":"Sa"}]}}]}`,
			`{"candidates":[{"content":{"parts":[{"text":"lon"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", data)
			f.Flush()
		}
	}))
	defer srv.Close()

	c := NewClient()
	es, err := c.Stream(context.Background(), UpstreamReq{
		Format: translate.FormatGemini, BaseURL: srv.URL, APIKey: "g-key",
		Internal: openaiReq("mock-gem", true), Stream: true,
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer es.Close()
	var text strings.Builder
	var sawEnd bool
	for {
		e, ok := es.Next()
		if !ok {
			break
		}
		if e.Type == translate.EventDelta {
			text.WriteString(e.Delta.Text)
		}
		if e.Type == translate.EventMessageEnd {
			sawEnd = true
		}
	}
	if text.String() != "Salon" || !sawEnd {
		t.Errorf("text=%q end=%v", text.String(), sawEnd)
	}
}

func TestUpstreamErrorParsing(t *testing.T) {
	// openai 429
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"Rate limit reached","type":"rate_limit_error","code":"rate_limit_exceeded"}}`)
	}))
	defer srv.Close()

	c := NewClient()
	_, err := c.Complete(context.Background(), UpstreamReq{
		Format: translate.FormatOpenAI, BaseURL: srv.URL, APIKey: "k",
		Internal: openaiReq("m", false),
	})
	ue, ok := err.(*translate.UpstreamError)
	if !ok {
		t.Fatalf("tipe = %T", err)
	}
	if ue.StatusCode != 429 || !ue.Retryable() || ue.Message != "Rate limit reached" {
		t.Fatalf("ue = %+v", ue)
	}
}

func TestCompleteTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		fmt.Fprint(w, `{"choices":[]}`)
	}))
	defer srv.Close()

	c := NewClient()
	_, err := c.Complete(context.Background(), UpstreamReq{
		Format: translate.FormatOpenAI, BaseURL: srv.URL, APIKey: "k",
		Internal: openaiReq("m", false), Timeout: 50 * time.Millisecond,
	})
	ue, ok := err.(*translate.UpstreamError)
	if !ok || !ue.Retryable() {
		t.Fatalf("timeout harus retryable: %v (%T)", err, err)
	}
}

func TestSSRFGuard(t *testing.T) {
	// loopback diblok untuk provider cloud
	if err := CheckBaseURL("http://127.0.0.1:8080/v1", false, false); err != ErrSSRFBlocked {
		t.Errorf("loopback harus diblokir: %v", err)
	}
	if err := CheckBaseURL("http://169.254.169.254/latest/meta-data", false, false); err != ErrSSRFBlocked {
		t.Errorf("metadata IP harus diblokir: %v", err)
	}
	if err := CheckBaseURL("http://10.0.0.5/v1", false, false); err != ErrSSRFBlocked {
		t.Errorf("privat harus diblokir: %v", err)
	}
	// LlamaStash lokal diizinkankan
	if err := CheckBaseURL("http://127.0.0.1:11435/v1", true, false); err != nil {
		t.Errorf("lokal harus diizinkan: %v", err)
	}
	// allowlist admin
	if err := CheckBaseURL("http://192.168.1.10:8000/v1", false, true); err != nil {
		t.Errorf("allow_private harus diizinkan: %v", err)
	}
	// skema aneh ditolak
	if err := CheckBaseURL("file:///etc/passwd", false, false); err == nil {
		t.Error("skema file harus ditolak")
	}
	if err := CheckBaseURL("http://", false, false); err == nil {
		t.Error("host kosong harus ditolak")
	}
	// publik lolos
	if err := CheckBaseURL("https://api.openai.com/v1", false, false); err != nil {
		t.Errorf("publik harus lolos: %v", err)
	}
}

func TestExtraHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "abc" {
			t.Errorf("header kustom hilang")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	c := NewClient()
	_, err := c.Complete(context.Background(), UpstreamReq{
		Format: translate.FormatOpenAI, BaseURL: srv.URL, APIKey: "k",
		Headers:  map[string]string{"X-Custom": "abc"},
		Internal: openaiReq("m", false),
	})
	if err != nil {
		t.Fatal(err)
	}
}
