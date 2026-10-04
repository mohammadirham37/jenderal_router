package translate

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ---------- Request ----------

// wireOpenAIRequest adalah bentuk wire OpenAI Chat Completions.
type wireOpenAIRequest struct {
	Model               string                `json:"model"`
	Messages            []wireOpenAIMessage   `json:"messages"`
	Tools               []wireOpenAITool      `json:"tools,omitempty"`
	ToolChoice          json.RawMessage       `json:"tool_choice,omitempty"`
	Temperature         *float64              `json:"temperature,omitempty"`
	TopP                *float64              `json:"top_p,omitempty"`
	MaxTokens           *int                  `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                  `json:"max_completion_tokens,omitempty"`
	Stop                json.RawMessage       `json:"stop,omitempty"`
	Stream              bool                  `json:"stream,omitempty"`
	StreamOptions       *wireStreamOptions    `json:"stream_options,omitempty"`
	ResponseFormat      *wireOpenAIRespFormat `json:"response_format,omitempty"`
	User                string                `json:"user,omitempty"`
	ParallelToolCalls   *bool                 `json:"parallel_tool_calls,omitempty"`
}

type wireStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type wireOpenAIMessage struct {
	Role       string               `json:"role"`
	Content    json.RawMessage      `json:"content,omitempty"`
	ToolCalls  []wireOpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
	Name       string               `json:"name,omitempty"`
}

type wireOpenAIToolCall struct {
	Index    *int              `json:"index,omitempty"`
	ID       string            `json:"id,omitempty"`
	Type     string            `json:"type,omitempty"` // "function"
	Function wireOpenAIFuncAll `json:"function"`
}

type wireOpenAIFuncAll struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type wireOpenAITool struct {
	Type     string            `json:"type"` // "function"
	Function wireOpenAIToolDef `json:"function"`
}

type wireOpenAIToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type wireOpenAIRespFormat struct {
	Type       string          `json:"type"`
	JSONSchema json.RawMessage `json:"json_schema,omitempty"`
}

// ParseOpenAIRequest mengubah body POST /v1/chat/completions menjadi
// ChatRequest internal. Field tak dikenal diabaikan (kompatibilitas klien).
func ParseOpenAIRequest(body []byte) (*ChatRequest, error) {
	var w wireOpenAIRequest
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("request bukan JSON OpenAI yang sah: %w", err)
	}
	req := &ChatRequest{
		Model:       w.Model,
		Stream:      w.Stream,
		User:        w.User,
		Temperature: w.Temperature,
		TopP:        w.TopP,
	}
	if w.MaxTokens != nil {
		req.MaxTokens = w.MaxTokens
	} else if w.MaxCompletionTokens != nil {
		req.MaxTokens = w.MaxCompletionTokens
	}
	if w.StreamOptions != nil {
		req.StreamUsage = w.StreamOptions.IncludeUsage
	}
	for _, m := range w.Messages {
		pm, err := parseOpenAIMessage(m)
		if err != nil {
			return nil, err
		}
		req.Messages = append(req.Messages, *pm)
	}
	for _, t := range w.Tools {
		req.Tools = append(req.Tools, Tool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		})
	}
	if len(w.ToolChoice) > 0 {
		tc, err := parseOpenAIToolChoice(w.ToolChoice)
		if err != nil {
			return nil, err
		}
		req.ToolChoice = tc
	}
	switch s := w.Stop; {
	case len(s) == 0:
	case s[0] == '"':
		var one string
		if err := json.Unmarshal(s, &one); err != nil {
			return nil, fmt.Errorf("stop tidak sah: %w", err)
		}
		req.Stop = []string{one}
	default:
		var many []string
		if err := json.Unmarshal(s, &many); err != nil {
			return nil, fmt.Errorf("stop tidak sah: %w", err)
		}
		req.Stop = many
	}
	if w.ResponseFormat != nil {
		req.RespFormat = &ResponseFormat{Type: w.ResponseFormat.Type, JSONSchema: w.ResponseFormat.JSONSchema}
	}
	return req, nil
}

func parseOpenAIMessage(m wireOpenAIMessage) (*Message, error) {
	msg := &Message{Role: m.Role, ToolCallID: m.ToolCallID, Name: m.Name}
	if len(m.Content) > 0 {
		parts, err := parseOpenAIContent(m.Content)
		if err != nil {
			return nil, err
		}
		msg.Content = parts
	}
	for _, tc := range m.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	return msg, nil
}

// parseOpenAIContent menerima string, null, atau array part.
func parseOpenAIContent(raw json.RawMessage) ([]Part, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" || trimmed == `""` {
		return nil, nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		if s == "" {
			return nil, nil
		}
		return []Part{{Type: PartText, Text: s}}, nil
	}
	var wireParts []wireOpenAIPart
	if err := json.Unmarshal(raw, &wireParts); err != nil {
		return nil, fmt.Errorf("content tidak sah: %w", err)
	}
	var parts []Part
	for _, wp := range wireParts {
		switch wp.Type {
		case "text", "refusal":
			parts = append(parts, Part{Type: PartText, Text: wp.Text})
		case "image_url":
			parts = append(parts, Part{Type: PartImage, ImageURL: wp.ImageURL.URL, Detail: wp.ImageURL.Detail})
		default:
			// part tidak dikenal → perlakukan sebagai teks bila ada
			if wp.Text != "" {
				parts = append(parts, Part{Type: PartText, Text: wp.Text})
			}
		}
	}
	return parts, nil
}

type wireOpenAIPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL struct {
		URL    string `json:"url"`
		Detail string `json:"detail,omitempty"`
	} `json:"image_url"`
}

func parseOpenAIToolChoice(raw json.RawMessage) (*ToolChoice, error) {
	t := strings.TrimSpace(string(raw))
	if t == `"auto"` || t == `""` {
		return &ToolChoice{Mode: ToolChoiceAuto}, nil
	}
	if t == `"none"` {
		return &ToolChoice{Mode: ToolChoiceNone}, nil
	}
	if t == `"required"` {
		return &ToolChoice{Mode: ToolChoiceRequired}, nil
	}
	var named struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &named); err != nil {
		return nil, fmt.Errorf("tool_choice tidak sah: %w", err)
	}
	return &ToolChoice{Mode: ToolChoiceNamed, Name: named.Function.Name}, nil
}

// RenderOpenAIRequest mengubah ChatRequest internal menjadi body JSON
// untuk upstream berformat OpenAI.
func RenderOpenAIRequest(req *ChatRequest) ([]byte, error) {
	w := wireOpenAIRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
		User:        req.User,
	}
	if req.Stream && req.StreamUsage {
		w.StreamOptions = &wireStreamOptions{IncludeUsage: true}
	}
	if req.MaxTokens != nil {
		w.MaxTokens = req.MaxTokens
	}
	for _, m := range req.Messages {
		wm := wireOpenAIMessage{Role: m.Role, ToolCallID: m.ToolCallID, Name: m.Name}
		wm.Content = renderOpenAIContent(m)
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireOpenAIToolCall{
				ID:       tc.ID,
				Type:     "function",
				Function: wireOpenAIFuncAll{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
		w.Messages = append(w.Messages, wm)
	}
	for _, t := range req.Tools {
		w.Tools = append(w.Tools, wireOpenAITool{
			Type:     "function",
			Function: wireOpenAIToolDef{Name: t.Name, Description: t.Description, Parameters: t.Parameters},
		})
	}
	if req.ToolChoice != nil {
		switch req.ToolChoice.Mode {
		case ToolChoiceAuto:
			w.ToolChoice = json.RawMessage(`"auto"`)
		case ToolChoiceNone:
			w.ToolChoice = json.RawMessage(`"none"`)
		case ToolChoiceRequired:
			w.ToolChoice = json.RawMessage(`"required"`)
		case ToolChoiceNamed:
			b, _ := json.Marshal(map[string]any{"type": "function", "function": map[string]string{"name": req.ToolChoice.Name}})
			w.ToolChoice = b
		}
	}
	if len(req.Stop) == 1 {
		w.Stop, _ = json.Marshal(req.Stop[0])
	} else if len(req.Stop) > 1 {
		w.Stop, _ = json.Marshal(req.Stop)
	}
	if req.RespFormat != nil {
		w.ResponseFormat = &wireOpenAIRespFormat{Type: req.RespFormat.Type, JSONSchema: req.RespFormat.JSONSchema}
	}
	return json.Marshal(w)
}

func renderOpenAIContent(m Message) json.RawMessage {
	// gunakan string untuk konten teks murni (paling kompatibel)
	if onlyText(m.Content) {
		return marshalString(m.TextContent())
	}
	var parts []map[string]any
	for _, p := range m.Content {
		switch p.Type {
		case PartText:
			parts = append(parts, map[string]any{"type": "text", "text": p.Text})
		case PartImage:
			iu := map[string]any{"url": p.ImageURL}
			if p.Detail != "" {
				iu["detail"] = p.Detail
			}
			parts = append(parts, map[string]any{"type": "image_url", "image_url": iu})
		}
	}
	if parts == nil {
		return json.RawMessage(`""`)
	}
	b, _ := json.Marshal(parts)
	return b
}

func onlyText(parts []Part) bool {
	for _, p := range parts {
		if p.Type != PartText {
			return false
		}
	}
	return true
}

func marshalString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// ---------- Respons non-streaming ----------

type wireOpenAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role      string               `json:"role"`
			Content   json.RawMessage      `json:"content"`
			ToolCalls []wireOpenAIToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireOpenAIUsage `json:"usage"`
	Error *wireOpenAIError `json:"error,omitempty"`
}

type wireOpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type wireOpenAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

// ParseOpenAIResponse mengubah respons JSON OpenAI (non-stream) menjadi
// ChatResponse internal; error upstream dikonversi menjadi *UpstreamError.
func ParseOpenAIResponse(body []byte) (*ChatResponse, error) {
	var w wireOpenAIResponse
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("respons upstream bukan JSON sah: %w", err)
	}
	if w.Error != nil {
		return nil, &UpstreamError{
			StatusCode: 200,
			Type:       w.Error.Type,
			Code:       fmt.Sprintf("%v", w.Error.Code),
			Message:    w.Error.Message,
		}
	}
	resp := &ChatResponse{ID: w.ID, Model: w.Model}
	if len(w.Choices) > 0 {
		c := w.Choices[0]
		parts, _ := parseOpenAIContent(orEmptyRaw(c.Message.Content))
		for _, p := range parts {
			if p.Type == PartText {
				resp.Content += p.Text
			}
		}
		for _, tc := range c.Message.ToolCalls {
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
		}
		if c.FinishReason != nil {
			resp.FinishReason = *c.FinishReason
		}
	}
	if w.Usage != nil {
		resp.Usage = Usage{PromptTokens: w.Usage.PromptTokens, CompletionTokens: w.Usage.CompletionTokens}
	}
	return resp, nil
}

func orEmptyRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`""`)
	}
	return raw
}

// RenderOpenAIResponse mengubah ChatResponse internal menjadi body JSON
// format OpenAI chat.completion (untuk klien /v1/chat/completions).
func RenderOpenAIResponse(r *ChatResponse) ([]byte, error) {
	content := marshalString(r.Content)
	var toolCalls []wireOpenAIToolCall
	for i, tc := range r.ToolCalls {
		toolCalls = append(toolCalls, wireOpenAIToolCall{
			Index:    intPtr(i),
			ID:       tc.ID,
			Type:     "function",
			Function: wireOpenAIFuncAll{Name: tc.Name, Arguments: tc.Arguments},
		})
	}
	resp := map[string]any{
		"id":      orDefault(r.ID, genID("chatcmpl")),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   r.Model,
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]any{
				"role":       "assistant",
				"content":    json.RawMessage(content),
				"tool_calls": toolCalls,
			},
			"finish_reason": orDefault(r.FinishReason, FinishStop),
		}},
		"usage": map[string]any{
			"prompt_tokens":     r.Usage.PromptTokens,
			"completion_tokens": r.Usage.CompletionTokens,
			"total_tokens":      r.Usage.Total(),
		},
	}
	return json.Marshal(resp)
}

// RenderOpenAIError membentuk body error format OpenAI (FR-3.4).
func RenderOpenAIError(status int, code, message string) []byte {
	b, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "invalid_request_error",
			"code":    code,
		},
	})
	return b
}

// ---------- Streaming SSE ----------

type wireOpenAIChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string               `json:"role,omitempty"`
			Content   json.RawMessage      `json:"content,omitempty"`
			ToolCalls []wireOpenAIToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireOpenAIUsage `json:"usage,omitempty"`
	Error *wireOpenAIError `json:"error,omitempty"`
}

// ParseOpenAISSEChunk mengubah payload `data:` OpenAI menjadi event internal.
// Satu chunk bisa menghasilkan beberapa event (delta, end, usage).
func ParseOpenAISSEChunk(data []byte) ([]Event, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "[DONE]" {
		return nil, nil
	}
	var w wireOpenAIChunk
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("chunk SSE OpenAI tidak sah: %w", err)
	}
	if w.Error != nil {
		return []Event{{
			Type:  EventError,
			Error: &UpstreamError{StatusCode: 200, Type: w.Error.Type, Code: codeString(w.Error.Code), Message: w.Error.Message},
		}}, nil
	}
	var events []Event
	if len(w.Choices) > 0 {
		c := w.Choices[0]
		delta := &StreamDelta{Role: c.Delta.Role}
		if len(c.Delta.Content) > 0 && string(c.Delta.Content) != "null" {
			parts, err := parseOpenAIContent(c.Delta.Content)
			if err == nil {
				for _, p := range parts {
					if p.Type == PartText {
						delta.Text += p.Text
					}
				}
			}
		}
		for _, tc := range c.Delta.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			delta.ToolCalls = append(delta.ToolCalls, ToolCallDelta{
				Index: idx, ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
			})
		}
		if c.Delta.Role != "" || delta.Text != "" || len(delta.ToolCalls) > 0 {
			events = append(events, Event{Type: EventDelta, Delta: delta, ID: w.ID, Model: w.Model})
		}
		if c.FinishReason != nil && *c.FinishReason != "" {
			events = append(events, Event{Type: EventMessageEnd, Delta: &StreamDelta{FinishReason: *c.FinishReason}, ID: w.ID, Model: w.Model})
		}
	}
	if w.Usage != nil && (w.Usage.PromptTokens > 0 || w.Usage.CompletionTokens > 0) {
		events = append(events, Event{Type: EventUsage, Usage: &Usage{
			PromptTokens:     w.Usage.PromptTokens,
			CompletionTokens: w.Usage.CompletionTokens,
		}})
	}
	return events, nil
}

func codeString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.Itoa(int(t))
	default:
		return fmt.Sprintf("%v", t)
	}
}

// RenderOpenAISSEChunk mengubah event internal menjadi payload `data:` OpenAI
// (tanpa awalan "data: ", tanpa newline). Return "" bila tak ada yang dikirim.
func RenderOpenAISSEChunk(e Event, id, model string) string {
	switch e.Type {
	case EventMessageBegin:
		w := map[string]any{
			"id": orDefault(id, genID("chatcmpl")), "object": "chat.completion.chunk",
			"created": time.Now().Unix(), "model": orDefault(model, e.Model),
			"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant", "content": ""}, "finish_reason": nil}},
		}
		return marshalCompact(w)
	case EventDelta:
		delta := map[string]any{}
		if e.Delta.Role != "" {
			delta["role"] = e.Delta.Role
		}
		if e.Delta.Text != "" {
			delta["content"] = e.Delta.Text
		}
		if len(e.Delta.ToolCalls) > 0 {
			var tcs []map[string]any
			for _, tc := range e.Delta.ToolCalls {
				fn := map[string]any{}
				if tc.Name != "" {
					fn["name"] = tc.Name
				}
				if tc.Arguments != "" {
					fn["arguments"] = tc.Arguments
				}
				m := map[string]any{"index": tc.Index, "function": fn}
				if tc.ID != "" {
					m["id"] = tc.ID
					m["type"] = "function"
				}
				tcs = append(tcs, m)
			}
			delta["tool_calls"] = tcs
		}
		if len(delta) == 0 {
			return ""
		}
		w := map[string]any{
			"id": orDefault(id, genID("chatcmpl")), "object": "chat.completion.chunk",
			"created": time.Now().Unix(), "model": orDefault(model, e.Model),
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": nil}},
		}
		return marshalCompact(w)
	case EventMessageEnd:
		w := map[string]any{
			"id": orDefault(id, genID("chatcmpl")), "object": "chat.completion.chunk",
			"created": time.Now().Unix(), "model": orDefault(model, e.Model),
			"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": orDefault(e.Delta.FinishReason, FinishStop)}},
		}
		return marshalCompact(w)
	case EventUsage:
		if e.Usage == nil {
			return ""
		}
		w := map[string]any{
			"id": orDefault(id, genID("chatcmpl")), "object": "chat.completion.chunk",
			"created": time.Now().Unix(), "model": orDefault(model, e.Model),
			"choices": []any{},
			"usage": map[string]any{
				"prompt_tokens":     e.Usage.PromptTokens,
				"completion_tokens": e.Usage.CompletionTokens,
				"total_tokens":      e.Usage.Total(),
			},
		}
		return marshalCompact(w)
	}
	return ""
}

func genID(prefix string) string {
	return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func marshalCompact(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func intPtr(i int) *int { return &i }
