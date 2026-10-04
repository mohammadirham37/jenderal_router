// Package translate melakukan translasi format chat dua arah antara
// skema internal JenderalRouter dan format provider: OpenAI Chat
// Completions, Anthropic Messages, dan Google Gemini generateContent
// (FR-3.3). Semua tipe di sini adalah kontrak inti gateway.
package translate

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Format menunjuk format wire protocol.
type Format string

const (
	FormatOpenAI    Format = "openai"
	FormatAnthropic Format = "anthropic"
	FormatGemini    Format = "gemini"
)

// Peran pesan.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Tipe part konten.
const (
	PartText  = "text"
	PartImage = "image"
)

// Part adalah segmen konten pesan (teks atau gambar untuk vision).
type Part struct {
	Type     string `json:"type"` // text | image
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"` // URL http(s) atau data: URL
	Detail   string `json:"detail,omitempty"`    // auto|low|high (OpenAI)
}

// ToolCall adalah pemanggilan tool lengkap (non-streaming).
type ToolCall struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // string JSON
}

// ToolCallDelta adalah potongan streaming tool call (OpenAI-style).
type ToolCallDelta struct {
	Index     int    `json:"index"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// Message adalah pesan chat ternormalisasi.
type Message struct {
	Role       string     `json:"role"`
	Content    []Part     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // untuk role=tool
	Name       string     `json:"name,omitempty"`
}

// TextContent menggabungkan seluruh part teks.
func (m *Message) TextContent() string {
	var b strings.Builder
	for _, p := range m.Content {
		if p.Type == PartText {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// Tool adalah definisi tool/function.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema
}

// ToolChoiceMode mode pemilihan tool.
type ToolChoiceMode string

const (
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceNamed    ToolChoiceMode = "tool"
)

// ToolChoice memilih perilaku tool calling.
type ToolChoice struct {
	Mode ToolChoiceMode `json:"mode"`
	Name string         `json:"name,omitempty"` // saat Mode=tool
}

// ResponseFormat memaksa bentuk keluaran (FR-3.1).
type ResponseFormat struct {
	Type       string          `json:"type"` // text | json_object
	JSONSchema json.RawMessage `json:"json_schema,omitempty"`
}

// ChatRequest adalah request chat ternormalisasi.
type ChatRequest struct {
	Model       string          `json:"model"`
	Messages    []Message       `json:"messages"`
	Tools       []Tool          `json:"tools,omitempty"`
	ToolChoice  *ToolChoice     `json:"tool_choice,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	StreamUsage bool            `json:"stream_usage,omitempty"` // stream_options.include_usage
	RespFormat  *ResponseFormat `json:"response_format,omitempty"`
	User        string          `json:"user,omitempty"`
}

// Alasan berhenti (internal, gaya OpenAI).
const (
	FinishStop          = "stop"
	FinishLength        = "length"
	FinishToolCalls     = "tool_calls"
	FinishContentFilter = "content_filter"
)

// Usage adalah pemakaian token; Estimated=true bila dihitung sendiri (FR-3.5).
type Usage struct {
	PromptTokens     int  `json:"prompt_tokens"`
	CompletionTokens int  `json:"completion_tokens"`
	Estimated        bool `json:"estimated,omitempty"`
}

// Total untuk kuota.
func (u Usage) Total() int { return u.PromptTokens + u.CompletionTokens }

// ChatResponse adalah respons non-streaming ternormalisasi.
type ChatResponse struct {
	ID           string     `json:"id"`
	Model        string     `json:"model"`
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason"`
	Usage        Usage      `json:"usage"`
}

// IsEmpty mendeteksi error konten kosong (pemicu fallback, FR-2.2).
func (r *ChatResponse) IsEmpty() bool {
	return strings.TrimSpace(r.Content) == "" && len(r.ToolCalls) == 0
}

// Tipe event stream internal.
type EventType string

const (
	EventMessageBegin EventType = "message_begin"
	EventDelta        EventType = "delta"
	EventUsage        EventType = "usage"
	EventMessageEnd   EventType = "message_end"
	EventError        EventType = "error"
)

// UpstreamError adalah error dari provider upstream.
type UpstreamError struct {
	StatusCode int    `json:"status_code"`
	Type       string `json:"type,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message"`
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("upstream %d %s: %s", e.StatusCode, e.Code, e.Message)
}

// Retryable menentukan apakah error layak fallback (FR-2.2):
// timeout/5xx/429/402 ya; 400 dan penolakan konten tidak.
func (e *UpstreamError) Retryable() bool {
	if e == nil {
		return false
	}
	switch {
	case e.StatusCode == 429, e.StatusCode == 402:
		return true
	case e.StatusCode >= 500:
		return true
	case e.StatusCode == 408, e.StatusCode == 0: // timeout / koneksi
		return true
	}
	return false
}

// StreamDelta adalah perubahan streaming.
type StreamDelta struct {
	Role         string          `json:"role,omitempty"`
	Text         string          `json:"text,omitempty"`
	ToolCalls    []ToolCallDelta `json:"tool_calls,omitempty"`
	FinishReason string          `json:"finish_reason,omitempty"`
}

// Event adalah satuan kejadian stream ternormalisasi.
type Event struct {
	Type  EventType      `json:"type"`
	Delta *StreamDelta   `json:"delta,omitempty"`
	Usage *Usage         `json:"usage,omitempty"`
	Error *UpstreamError `json:"error,omitempty"`
	Model string         `json:"model,omitempty"`
	ID    string         `json:"id,omitempty"`
}

// EstimateTokens mengestimasi jumlah token teks (FR-3.5, flag estimated).
// Heuristik BPE-lite: ~4 karakter per token untuk Latin, dikoreksi untuk
// karakter non-Latin (~1,5 karakter per token) dan tanda baca/whitespace.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	var latin, other int
	for _, r := range text {
		if r < 0x2E80 { // ASCII/Latin/tanda baca umum
			latin++
		} else {
			other++
		}
	}
	tokens := latin/4 + (other*2)/3
	if tokens == 0 {
		tokens = 1
	}
	return tokens
}

// EstimateMessages mengestimasi token seluruh percakapan + overhead per pesan.
func EstimateMessages(msgs []Message) int {
	total := 0
	for _, m := range msgs {
		total += EstimateTokens(m.TextContent()) + 4
		for _, tc := range m.ToolCalls {
			total += EstimateTokens(tc.Name+tc.Arguments) + 3
		}
	}
	return total
}
