// Package provider berisi adapter menuju provider upstream (cloud atau
// LlamaStash lokal): membangun request HTTP sesuai format, mem-parsing
// respons/SSE menjadi event internal, serta menjaga SSRF (NFR-07).
package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jenderal/jenderalrouter/internal/translate"
)

// Client mengirim request ke satu provider upstream.
type Client struct {
	HTTP *http.Client
}

// NewClient membuat client dengan transport aman default.
func NewClient() *Client {
	return &Client{
		HTTP: &http.Client{
			// Timeout dikelola per-request lewat context.
			Transport: &http.Transport{
				MaxIdleConns:        200,
				MaxIdleConnsPerHost: 50,
				IdleConnTimeout:     90 * time.Second,
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
		},
	}
}

// UpstreamReq deskripsi satu panggilan upstream.
type UpstreamReq struct {
	Format   translate.Format
	BaseURL  string
	APIKey   string
	Headers  map[string]string
	Internal *translate.ChatRequest // req.Model harus nama upstream
	Stream   bool
	Timeout  time.Duration // 0 = tanpa batas total (untuk streaming)
}

// EventStream iterasi event internal dari upstream.
type EventStream struct {
	events chan translate.Event
	cancel context.CancelFunc
	// final usage tercatat selama streaming
	Usage translate.Usage
}

// Events mengembalikan channel baca event (untuk pemakai luar paket).
func (es *EventStream) Events() <-chan translate.Event { return es.events }

// Next menunggu event berikutnya; false bila stream selesai.
func (es *EventStream) Next() (translate.Event, bool) {
	e, ok := <-es.events
	return e, ok
}

// Close menghentikan stream dan koneksi upstream.
func (es *EventStream) Close() {
	if es.cancel != nil {
		es.cancel()
	}
	// kuras channel agar goroutine producer selesai
	for range es.events {
	}
}

// Complete melakukan panggilan non-streaming.
func (c *Client) Complete(ctx context.Context, r UpstreamReq) (*translate.ChatResponse, error) {
	body, err := buildRequestBody(r)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, r, body)
	if err != nil {
		return nil, err
	}
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
		httpReq = httpReq.WithContext(ctx)
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, classifyTransport(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, &translate.UpstreamError{StatusCode: 0, Message: fmt.Sprintf("baca respons: %v", err)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseUpstreamError(r.Format, resp.StatusCode, data)
	}
	switch r.Format {
	case translate.FormatAnthropic:
		return translate.ParseAnthropicResponse(data)
	case translate.FormatGemini:
		return translate.ParseGeminiResponse(data)
	default:
		return translate.ParseOpenAIResponse(data)
	}
}

// Stream memulai panggilan streaming; mengembalikan stream event internal.
// Error sebelum token pertama dikembalikan sebagai return value kedua.
// Timeout (bila > 0) hanya membatasi waktu sampai respons header (first byte)
// agar stream panjang tidak terpotong; pembatalan penuh lewat EventStream.Close.
func (c *Client) Stream(ctx context.Context, r UpstreamReq) (*EventStream, error) {
	body, err := buildRequestBody(r)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, r, body)
	if err != nil {
		return nil, err
	}
	client := c.HTTP
	if r.Timeout > 0 {
		tr := c.HTTP.Transport.(*http.Transport).Clone()
		tr.ResponseHeaderTimeout = r.Timeout
		client = &http.Client{Transport: tr}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, classifyTransport(err)
	}
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		return nil, parseUpstreamError(r.Format, resp.StatusCode, data)
	}

	ctx, cancel := context.WithCancel(ctx)
	es := &EventStream{events: make(chan translate.Event, 64), cancel: cancel}
	switch r.Format {
	case translate.FormatAnthropic:
		go es.consumeAnthropic(resp.Body)
	case translate.FormatGemini:
		go es.consumeGemini(resp.Body)
	default:
		go es.consumeOpenAI(resp.Body)
	}
	return es, nil
}

func buildRequestBody(r UpstreamReq) ([]byte, error) {
	switch r.Format {
	case translate.FormatAnthropic:
		return translate.RenderAnthropicRequest(r.Internal)
	case translate.FormatGemini:
		return translate.RenderGeminiRequest(r.Internal)
	default:
		return translate.RenderOpenAIRequest(r.Internal)
	}
}

func (c *Client) newRequest(ctx context.Context, r UpstreamReq, body []byte) (*http.Request, error) {
	var endpoint string
	switch r.Format {
	case translate.FormatAnthropic:
		endpoint = strings.TrimRight(r.BaseURL, "/") + "/v1/messages"
	case translate.FormatGemini:
		endpoint = translate.GeminiURL(r.BaseURL, r.Internal.Model, r.Stream)
	default:
		endpoint = strings.TrimRight(r.BaseURL, "/") + "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	switch r.Format {
	case translate.FormatAnthropic:
		req.Header.Set("x-api-key", r.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case translate.FormatGemini:
		req.Header.Set("x-goog-api-key", r.APIKey)
	default:
		if r.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+r.APIKey)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// ---- konsumsi SSE per format ----

func (es *EventStream) consumeOpenAI(body io.Reader) {
	defer close(es.events)
	promptTokens, completionTokens := es.consumeSSE(body, func(data []byte) ([]translate.Event, error) {
		return translate.ParseOpenAISSEChunk(data)
	})
	es.Usage = translate.Usage{PromptTokens: promptTokens, CompletionTokens: completionTokens}
}

func (es *EventStream) consumeAnthropic(body io.Reader) {
	defer close(es.events)
	parser := translate.NewAnthropicSSEParser()
	var in, out int
	es.consumeSSE(body, func(data []byte) ([]translate.Event, error) {
		evs, err := parser.Parse(data)
		for _, e := range evs {
			if e.Type == translate.EventUsage && e.Usage != nil {
				in, out = e.Usage.PromptTokens, e.Usage.CompletionTokens
			}
		}
		return evs, err
	})
	es.Usage = translate.Usage{PromptTokens: in, CompletionTokens: out}
}

func (es *EventStream) consumeGemini(body io.Reader) {
	defer close(es.events)
	var in, out int
	es.consumeSSE(body, func(data []byte) ([]translate.Event, error) {
		evs, err := translate.ParseGeminiSSEChunk(data)
		for _, e := range evs {
			if e.Type == translate.EventUsage && e.Usage != nil {
				in, out = e.Usage.PromptTokens, e.Usage.CompletionTokens
			}
		}
		return evs, err
	})
	es.Usage = translate.Usage{PromptTokens: in, CompletionTokens: out}
}

// consumeSSE membaca baris `data:` dari body dan memanggil parse per payload.
// mengembalikan usage terakhir yang terlihat.
func (es *EventStream) consumeSSE(body io.Reader, parse func([]byte) ([]translate.Event, error)) (int, int) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var in, out int
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		evs, err := parse([]byte(payload))
		if err != nil {
			// chunk rusak: laporkan sebagai error lalu hentikan
			es.events <- translate.Event{Type: translate.EventError, Error: &translate.UpstreamError{
				StatusCode: 200, Message: err.Error(),
			}}
			return in, out
		}
		for _, e := range evs {
			if e.Type == translate.EventUsage && e.Usage != nil {
				in, out = e.Usage.PromptTokens, e.Usage.CompletionTokens
			}
			select {
			case es.events <- e:
			case <-time.After(30 * time.Second):
				return in, out
			}
		}
	}
	return in, out
}

// ---- error helpers ----

// classifyTransport mengubah error transport menjadi UpstreamError retryable.
func classifyTransport(err error) *translate.UpstreamError {
	if ue, ok := err.(*translate.UpstreamError); ok {
		return ue
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &translate.UpstreamError{StatusCode: 408, Message: "timeout menghubungi provider"}
	}
	if errors.Is(err, context.Canceled) {
		return &translate.UpstreamError{StatusCode: 499, Message: "dibatalkan klien"}
	}
	return &translate.UpstreamError{StatusCode: 0, Message: "koneksi provider gagal: " + err.Error()}
}

// parseUpstreamError menormalkan body error provider sesuai format (FR-2.2).
func parseUpstreamError(f translate.Format, status int, body []byte) *translate.UpstreamError {
	msg := strings.TrimSpace(string(body))
	if len(msg) > 500 {
		msg = msg[:500]
	}
	code := ""
	typ := ""
	var generic struct {
		Error jsonError `json:"error"`
	}
	var anthropicErr struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	switch f {
	case translate.FormatAnthropic:
		if err := jsonUnmarshal(body, &anthropicErr); err == nil && anthropicErr.Error.Message != "" {
			msg = anthropicErr.Error.Message
			typ = anthropicErr.Error.Type
		}
	case translate.FormatGemini:
		var gerr struct {
			Error struct {
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		if err := jsonUnmarshal(body, &gerr); err == nil && gerr.Error.Message != "" {
			msg = gerr.Error.Message
			typ = gerr.Error.Status
		}
	default:
		if err := jsonUnmarshal(body, &generic); err == nil && generic.Error.Message != "" {
			msg = generic.Error.Message
			typ = generic.Error.Type
			switch cv := generic.Error.Code.(type) {
			case string:
				code = cv
			case float64:
				code = fmt.Sprintf("%d", int(cv))
			}
		}
	}
	return &translate.UpstreamError{StatusCode: status, Type: typ, Code: code, Message: msg}
}

type jsonError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// ---- SSRF guard (NFR-07) ----

// ErrSSRFBlocked dilempar bila base URL mengarah ke jaringan privat.
var ErrSSRFBlocked = errors.New("base URL provider menunjuk jaringan privat/loopback (diblokir SSRF guard)")

// CheckBaseURL memvalidasi base URL provider: skema http(s), host aman.
// Loopback/link-local/privat diblokir kecuali isLocal (LlamaStash) atau
// admin menyetel ssrf_allow_private.
func CheckBaseURL(baseURL string, isLocal, allowPrivate bool) error {
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("base URL tidak sah: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("base URL harus http/https, dapat %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("base URL tanpa host")
	}
	if isLocal || allowPrivate {
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// host tak terresolvo → biarkan client yang melaporkan saat connect
		return nil
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return ErrSSRFBlocked
		}
	}
	return nil
}
