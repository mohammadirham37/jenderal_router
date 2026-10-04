package translate

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ---------- Request (Anthropic Messages → internal) ----------

type wireAnthropicRequest struct {
	Model         string               `json:"model"`
	MaxTokens     int                  `json:"max_tokens"`
	Messages      []wireAnthropicMsg   `json:"messages"`
	System        json.RawMessage      `json:"system,omitempty"`
	Temperature   *float64             `json:"temperature,omitempty"`
	TopP          *float64             `json:"top_p,omitempty"`
	TopK          *int                 `json:"top_k,omitempty"`
	StopSequences []string             `json:"stop_sequences,omitempty"`
	Stream        bool                 `json:"stream,omitempty"`
	Tools         []wireAnthropicTool  `json:"tools,omitempty"`
	ToolChoice    *wireAnthropicChoice `json:"tool_choice,omitempty"`
	Metadata      json.RawMessage      `json:"metadata,omitempty"`
}

type wireAnthropicMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type wireAnthropicBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// image
	Source *wireAnthropicSource `json:"source,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type wireAnthropicSource struct {
	Type      string `json:"type"` // base64 | url
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type wireAnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type wireAnthropicChoice struct {
	Type string `json:"type"` // auto | any | tool
	Name string `json:"name,omitempty"`
}

// ParseAnthropicRequest mengubah body POST /v1/messages menjadi ChatRequest.
func ParseAnthropicRequest(body []byte) (*ChatRequest, error) {
	var w wireAnthropicRequest
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("request bukan JSON Anthropic yang sah: %w", err)
	}
	if w.Model == "" {
		return nil, fmt.Errorf("field model wajib")
	}
	if w.MaxTokens <= 0 {
		return nil, fmt.Errorf("field max_tokens wajib dan > 0")
	}
	req := &ChatRequest{
		Model:       w.Model,
		MaxTokens:   &w.MaxTokens,
		Stream:      w.Stream,
		Temperature: w.Temperature,
		TopP:        w.TopP,
		Stop:        w.StopSequences,
	}
	// system: string atau array blok teks
	if len(w.System) > 0 && string(w.System) != "null" {
		sys := ""
		if w.System[0] == '"' {
			json.Unmarshal(w.System, &sys)
		} else {
			var blocks []wireAnthropicBlock
			if err := json.Unmarshal(w.System, &blocks); err == nil {
				var b strings.Builder
				for _, bl := range blocks {
					if bl.Type == "text" {
						b.WriteString(bl.Text)
					}
				}
				sys = b.String()
			}
		}
		if sys != "" {
			req.Messages = append(req.Messages, Message{Role: RoleSystem, Content: []Part{{Type: PartText, Text: sys}}})
		}
	}
	for _, m := range w.Messages {
		msgs, err := parseAnthropicMessage(m)
		if err != nil {
			return nil, err
		}
		req.Messages = append(req.Messages, msgs...)
	}
	for _, tl := range w.Tools {
		req.Tools = append(req.Tools, Tool{Name: tl.Name, Description: tl.Description, Parameters: tl.InputSchema})
	}
	if w.ToolChoice != nil {
		switch w.ToolChoice.Type {
		case "auto":
			req.ToolChoice = &ToolChoice{Mode: ToolChoiceAuto}
		case "any":
			req.ToolChoice = &ToolChoice{Mode: ToolChoiceRequired}
		case "tool":
			req.ToolChoice = &ToolChoice{Mode: ToolChoiceNamed, Name: w.ToolChoice.Name}
		}
	}
	return req, nil
}

func parseAnthropicMessage(m wireAnthropicMsg) ([]Message, error) {
	trimmed := strings.TrimSpace(string(m.Content))
	if trimmed == "" || trimmed == "null" {
		return []Message{{Role: m.Role}}, nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(m.Content, &s); err != nil {
			return nil, err
		}
		return []Message{{Role: m.Role, Content: []Part{{Type: PartText, Text: s}}}}, nil
	}
	var blocks []wireAnthropicBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return nil, fmt.Errorf("content Anthropic tidak sah: %w", err)
	}
	var out []Message
	var currentParts []Part
	flush := func() {
		if len(currentParts) > 0 {
			out = append(out, Message{Role: m.Role, Content: currentParts})
			currentParts = nil
		}
	}
	for _, bl := range blocks {
		switch bl.Type {
		case "text":
			currentParts = append(currentParts, Part{Type: PartText, Text: bl.Text})
		case "image":
			url := ""
			if bl.Source != nil {
				switch bl.Source.Type {
				case "base64":
					url = "data:" + bl.Source.MediaType + ";base64," + bl.Source.Data
				case "url":
					url = bl.Source.URL
				}
			}
			currentParts = append(currentParts, Part{Type: PartImage, ImageURL: url})
		case "tool_use":
			flush()
			args := string(bl.Input)
			if args == "" {
				args = "{}"
			}
			out = append(out, Message{
				Role:      RoleAssistant,
				ToolCalls: []ToolCall{{ID: bl.ID, Name: bl.Name, Arguments: args}},
			})
		case "tool_result":
			flush()
			text := toolResultText(bl.Content)
			out = append(out, Message{Role: RoleTool, ToolCallID: bl.ToolUseID, Content: []Part{{Type: PartText, Text: text}}})
		}
	}
	flush()
	if len(out) == 0 {
		out = append(out, Message{Role: m.Role})
	}
	return out, nil
}

// toolResultText menormalkan content tool_result (string atau blok) ke teks.
func toolResultText(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	if trimmed[0] == '"' {
		var s string
		json.Unmarshal(raw, &s)
		return s
	}
	var blocks []wireAnthropicBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var b strings.Builder
		for _, bl := range blocks {
			if bl.Type == "text" {
				b.WriteString(bl.Text)
			}
		}
		return b.String()
	}
	return trimmed
}

// ---------- Request (internal → Anthropic) ----------

// RenderAnthropicRequest mengubah ChatRequest menjadi body JSON untuk
// upstream Anthropic. Pesan role tool dikonversi ke blok tool_result di
// pesan user, dan pesan berurutan dengan role sama digabung (syarat API
// Anthropic: user/assistant harus bergantian).
func RenderAnthropicRequest(req *ChatRequest) ([]byte, error) {
	w := wireAnthropicRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}
	if req.MaxTokens != nil {
		w.MaxTokens = *req.MaxTokens
	} else {
		w.MaxTokens = 4096 // Anthropic mewajibkan max_tokens
	}
	w.StopSequences = req.Stop

	var systemParts []Part
	// gabung pesan: tool → blok tool_result dalam user
	type accMsg struct {
		role   string
		blocks []wireAnthropicBlock
	}
	var acc []accMsg
	appendBlock := func(role string, bl wireAnthropicBlock) {
		if len(acc) > 0 && acc[len(acc)-1].role == role {
			acc[len(acc)-1].blocks = append(acc[len(acc)-1].blocks, bl)
		} else {
			acc = append(acc, accMsg{role: role, blocks: []wireAnthropicBlock{bl}})
		}
	}
	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			systemParts = append(systemParts, m.Content...)
		case RoleUser:
			for _, p := range m.Content {
				if p.Type == PartImage {
					bl := wireAnthropicBlock{Type: "image"}
					if strings.HasPrefix(p.ImageURL, "data:") {
						media, data := parseDataURL(p.ImageURL)
						bl.Source = &wireAnthropicSource{Type: "base64", MediaType: media, Data: data}
					} else {
						bl.Source = &wireAnthropicSource{Type: "url", URL: p.ImageURL}
					}
					appendBlock("user", bl)
				} else if p.Text != "" {
					appendBlock("user", wireAnthropicBlock{Type: "text", Text: p.Text})
				}
			}
		case RoleTool:
			appendBlock("user", wireAnthropicBlock{Type: "tool_result", ToolUseID: m.ToolCallID,
				Content: json.RawMessage(marshalString(m.TextContent()))})
		case RoleAssistant:
			text := m.TextContent()
			if text != "" {
				appendBlock("assistant", wireAnthropicBlock{Type: "text", Text: text})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(tc.Arguments)
				if !json.Valid(input) {
					input = json.RawMessage(`{}`)
				}
				appendBlock("assistant", wireAnthropicBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input})
			}
		}
	}
	if len(systemParts) > 0 {
		sys := ""
		for i, p := range systemParts {
			if i > 0 {
				sys += "\n"
			}
			sys += p.Text
		}
		w.System = json.RawMessage(marshalString(sys))
	}
	for _, m := range acc {
		b, _ := json.Marshal(m.blocks)
		w.Messages = append(w.Messages, wireAnthropicMsg{Role: m.role, Content: b})
	}
	// tool_choice=none → buang tools (Anthropic tidak punya mode none)
	if req.ToolChoice != nil && req.ToolChoice.Mode == ToolChoiceNone {
		req = &ChatRequest{Model: req.Model, Messages: req.Messages, MaxTokens: req.MaxTokens,
			Temperature: req.Temperature, TopP: req.TopP, Stop: req.Stop, Stream: req.Stream}
	} else {
		for _, tl := range req.Tools {
			schema := tl.Parameters
			if len(schema) == 0 {
				schema = json.RawMessage(`{"type":"object"}`)
			}
			w.Tools = append(w.Tools, wireAnthropicTool{Name: tl.Name, Description: tl.Description, InputSchema: schema})
		}
		if req.ToolChoice != nil {
			switch req.ToolChoice.Mode {
			case ToolChoiceAuto:
				w.ToolChoice = &wireAnthropicChoice{Type: "auto"}
			case ToolChoiceRequired:
				w.ToolChoice = &wireAnthropicChoice{Type: "any"}
			case ToolChoiceNamed:
				w.ToolChoice = &wireAnthropicChoice{Type: "tool", Name: req.ToolChoice.Name}
			}
		}
	}
	return json.Marshal(w)
}

func parseDataURL(u string) (media, data string) {
	// data:<media>;base64,<data>
	rest := strings.TrimPrefix(u, "data:")
	idx := strings.Index(rest, ";base64,")
	if idx < 0 {
		return "application/octet-stream", rest
	}
	return rest[:idx], rest[idx+len(";base64,"):]
}

// ---------- Respons non-streaming ----------

type wireAnthropicResponse struct {
	ID         string                `json:"id"`
	Type       string                `json:"type"`
	Role       string                `json:"role"`
	Model      string                `json:"model"`
	Content    []wireAnthropicBlock  `json:"content"`
	StopReason string                `json:"stop_reason"`
	Usage      wireAnthropicUsage    `json:"usage"`
	Error      *wireAnthropicErrBody `json:"error,omitempty"`
}

type wireAnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type wireAnthropicErrBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// StopReasonAnthropicToInternal memetakan stop_reason Anthropic → internal.
func StopReasonAnthropicToInternal(s string) string {
	switch s {
	case "end_turn", "stop_sequence":
		return FinishStop
	case "max_tokens":
		return FinishLength
	case "tool_use":
		return FinishToolCalls
	case "refusal":
		return FinishContentFilter
	default:
		return FinishStop
	}
}

// StopReasonInternalToAnthropic memetakan internal → Anthropic.
func StopReasonInternalToAnthropic(s string) string {
	switch s {
	case FinishLength:
		return "max_tokens"
	case FinishToolCalls:
		return "tool_use"
	case FinishContentFilter:
		return "refusal"
	default:
		return "end_turn"
	}
}

// ParseAnthropicResponse mengubah respons JSON Anthropic non-stream.
func ParseAnthropicResponse(body []byte) (*ChatResponse, error) {
	var w wireAnthropicResponse
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("respons upstream bukan JSON Anthropic sah: %w", err)
	}
	if w.Error != nil {
		return nil, &UpstreamError{StatusCode: 200, Type: w.Error.Type, Message: w.Error.Message}
	}
	resp := &ChatResponse{ID: w.ID, Model: w.Model}
	var b strings.Builder
	for _, bl := range w.Content {
		switch bl.Type {
		case "text":
			b.WriteString(bl.Text)
		case "tool_use":
			args := string(bl.Input)
			if args == "" || !json.Valid(json.RawMessage(args)) {
				args = "{}"
			}
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: bl.ID, Name: bl.Name, Arguments: args})
		}
	}
	resp.Content = b.String()
	resp.FinishReason = StopReasonAnthropicToInternal(w.StopReason)
	resp.Usage = Usage{PromptTokens: w.Usage.InputTokens, CompletionTokens: w.Usage.OutputTokens}
	return resp, nil
}

// RenderAnthropicResponse mengubah ChatResponse internal menjadi body JSON
// format Anthropic message (untuk klien /v1/messages).
func RenderAnthropicResponse(r *ChatResponse) ([]byte, error) {
	var blocks []wireAnthropicBlock
	if r.Content != "" {
		blocks = append(blocks, wireAnthropicBlock{Type: "text", Text: r.Content})
	}
	for _, tc := range r.ToolCalls {
		input := json.RawMessage(tc.Arguments)
		if !json.Valid(input) {
			input = json.RawMessage(`{}`)
		}
		blocks = append(blocks, wireAnthropicBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input})
	}
	if len(blocks) == 0 {
		blocks = []wireAnthropicBlock{{Type: "text", Text: ""}}
	}
	resp := map[string]any{
		"id":            orDefault(r.ID, "msg_"+orDefault(r.ID, genID("jr"))),
		"type":          "message",
		"role":          "assistant",
		"model":         r.Model,
		"content":       blocks,
		"stop_reason":   StopReasonInternalToAnthropic(r.FinishReason),
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  r.Usage.PromptTokens,
			"output_tokens": r.Usage.CompletionTokens,
		},
	}
	return json.Marshal(resp)
}

// RenderAnthropicError membentuk body error format Anthropic (FR-3.4).
func RenderAnthropicError(errType, message string) []byte {
	b, _ := json.Marshal(map[string]any{
		"type":  "error",
		"error": map[string]any{"type": errType, "message": message},
	})
	return b
}

// MapStatusToAnthropicErrorType memilih tipe error Anthropic dari status HTTP.
func MapStatusToAnthropicErrorType(status int) string {
	switch {
	case status == 400:
		return "invalid_request_error"
	case status == 401:
		return "authentication_error"
	case status == 403:
		return "permission_error"
	case status == 404:
		return "not_found_error"
	case status == 429:
		return "rate_limit_error"
	case status >= 500:
		return "api_error"
	default:
		return "api_error"
	}
}

// ---------- Streaming SSE Anthropic ----------

type wireAnthropicSSE struct {
	Type    string `json:"type"`
	Index   int    `json:"index,omitempty"`
	Message *struct {
		ID    string             `json:"id"`
		Model string             `json:"model"`
		Usage wireAnthropicUsage `json:"usage"`
	} `json:"message,omitempty"`
	ContentBlock *wireAnthropicBlock   `json:"content_block,omitempty"`
	Delta        *wireAnthropicDelta   `json:"delta,omitempty"`
	Usage        *wireAnthropicUsage   `json:"usage,omitempty"`
	Error        *wireAnthropicErrBody `json:"error,omitempty"`
}

type wireAnthropicDelta struct {
	Type        string `json:"type"` // text_delta | input_json_delta
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

// AnthropicSSEParser mengubah rangkaian payload `data:` Anthropic menjadi
// event internal; stateful (mengingat blok per index).
type AnthropicSSEParser struct {
	blockTypes map[int]string
	inputTok   int
	outputTok  int
	id, model  string
	sawEnd     bool
}

// NewAnthropicSSEParser membuat parser baru.
func NewAnthropicSSEParser() *AnthropicSSEParser {
	return &AnthropicSSEParser{blockTypes: map[int]string{}}
}

// Parse mengonversi satu payload data menjadi event.
func (p *AnthropicSSEParser) Parse(data []byte) ([]Event, error) {
	var w wireAnthropicSSE
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("event SSE Anthropic tidak sah: %w", err)
	}
	var events []Event
	switch w.Type {
	case "ping":
		// abaikan
	case "message_start":
		if w.Message != nil {
			p.id = w.Message.ID
			p.model = w.Message.Model
			p.inputTok = w.Message.Usage.InputTokens
			events = append(events, Event{Type: EventMessageBegin, ID: w.Message.ID, Model: w.Message.Model})
		}
	case "content_block_start":
		if w.ContentBlock != nil && w.ContentBlock.Type == "tool_use" {
			p.blockTypes[w.Index] = "tool_use"
			events = append(events, Event{Type: EventDelta, Delta: &StreamDelta{
				ToolCalls: []ToolCallDelta{{Index: w.Index, ID: w.ContentBlock.ID, Name: w.ContentBlock.Name}},
			}})
		} else {
			p.blockTypes[w.Index] = "text"
		}
	case "content_block_delta":
		if w.Delta == nil {
			break
		}
		switch w.Delta.Type {
		case "text_delta":
			events = append(events, Event{Type: EventDelta, Delta: &StreamDelta{Text: w.Delta.Text}})
		case "input_json_delta":
			events = append(events, Event{Type: EventDelta, Delta: &StreamDelta{
				ToolCalls: []ToolCallDelta{{Index: w.Index, Arguments: w.Delta.PartialJSON}},
			}})
		}
	case "message_delta":
		if w.Delta != nil && w.Delta.StopReason != "" {
			p.sawEnd = true
			events = append(events, Event{Type: EventMessageEnd, Delta: &StreamDelta{
				FinishReason: StopReasonAnthropicToInternal(w.Delta.StopReason),
			}})
		}
		if w.Usage != nil {
			p.outputTok = w.Usage.OutputTokens
			events = append(events, Event{Type: EventUsage, Usage: &Usage{
				PromptTokens: p.inputTok, CompletionTokens: w.Usage.OutputTokens,
			}})
		}
	case "message_stop":
		if !p.sawEnd {
			p.sawEnd = true
			events = append(events, Event{Type: EventMessageEnd, Delta: &StreamDelta{FinishReason: FinishStop}})
		}
	case "error":
		msg := "unknown error"
		typ := "api_error"
		if w.Error != nil {
			msg = w.Error.Message
			typ = w.Error.Type
		}
		events = append(events, Event{Type: EventError, Error: &UpstreamError{StatusCode: 200, Type: typ, Message: msg}})
	}
	return events, nil
}

// AnthropicSSEWriter merender event internal menjadi frame SSE Anthropic
// (tiap frame berformat "event: <t>\ndata: <json>\n\n"); stateful.
type AnthropicSSEWriter struct {
	msgID     string
	model     string
	nextIndex int
	openIndex int // indeks blok terbuka; -1 bila tak ada
	openTool  bool
	inputTok  int
	outputTok int
	endSent   bool
	stopped   bool
}

// NewAnthropicSSEWriter membuat writer baru.
func NewAnthropicSSEWriter() *AnthropicSSEWriter {
	return &AnthropicSSEWriter{openIndex: -1}
}

func frame(event string, v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return "event: " + event + "\ndata: " + string(b) + "\n\n"
}

// Begin menghasilkan frame message_start.
func (w *AnthropicSSEWriter) Begin(id, model string, inputTokens int) string {
	w.msgID = orDefault(id, "msg_"+genID("jr"))
	w.model = model
	w.inputTok = inputTokens
	return frame("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": w.msgID, "type": "message", "role": "assistant", "model": model,
			"content":     []any{},
			"stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": inputTokens, "output_tokens": 0},
		},
	})
}

// WriteEvent merender satu event internal ke 0..n frame SSE.
func (w *AnthropicSSEWriter) WriteEvent(e Event) []string {
	var out []string
	switch e.Type {
	case EventMessageBegin:
		return []string{w.Begin(e.ID, e.Model, 0)}
	case EventDelta:
		if e.Delta == nil {
			return nil
		}
		if e.Delta.Text != "" {
			if w.openIndex < 0 || w.openTool {
				if w.openIndex >= 0 {
					out = append(out, w.closeBlock())
				}
				w.openIndex = w.nextIndex
				w.nextIndex++
				w.openTool = false
				out = append(out, frame("content_block_start", map[string]any{
					"type": "content_block_start", "index": w.openIndex,
					"content_block": map[string]any{"type": "text", "text": ""},
				}))
			}
			out = append(out, frame("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": w.openIndex,
				"delta": map[string]any{"type": "text_delta", "text": e.Delta.Text},
			}))
		}
		for _, tc := range e.Delta.ToolCalls {
			out = append(out, w.writeToolDelta(tc)...)
		}
	case EventUsage:
		if e.Usage != nil {
			w.inputTok = e.Usage.PromptTokens
			w.outputTok = e.Usage.CompletionTokens
		}
	case EventMessageEnd:
		out = append(out, w.writeEnd(e)...)
	}
	return out
}

func (w *AnthropicSSEWriter) writeToolDelta(tc ToolCallDelta) []string {
	var out []string
	if w.openIndex >= 0 && (w.openIndex != tc.Index || !w.openTool) {
		out = append(out, w.closeBlock())
	}
	if w.openIndex != tc.Index {
		w.openIndex = tc.Index
		w.openTool = true
		if tc.Index >= w.nextIndex {
			w.nextIndex = tc.Index + 1
		}
		out = append(out, frame("content_block_start", map[string]any{
			"type": "content_block_start", "index": tc.Index,
			"content_block": map[string]any{"type": "tool_use", "id": tc.ID, "name": tc.Name, "input": map[string]any{}},
		}))
	}
	if tc.Arguments != "" {
		out = append(out, frame("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": tc.Index,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": tc.Arguments},
		}))
	}
	return out
}

func (w *AnthropicSSEWriter) closeBlock() string {
	idx := w.openIndex
	w.openIndex = -1
	w.openTool = false
	return frame("content_block_stop", map[string]any{"type": "content_block_stop", "index": idx})
}

func (w *AnthropicSSEWriter) writeEnd(e Event) []string {
	if w.endSent {
		return nil
	}
	w.endSent = true
	var out []string
	if w.openIndex >= 0 {
		out = append(out, w.closeBlock())
	}
	reason := FinishStop
	if e.Delta != nil && e.Delta.FinishReason != "" {
		reason = e.Delta.FinishReason
	}
	if e.Usage != nil {
		w.outputTok = e.Usage.CompletionTokens
	}
	out = append(out, frame("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": StopReasonInternalToAnthropic(reason), "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": w.outputTok},
	}))
	return out
}

// Finish menutup stream (message_stop) dan mengembalikan frame penutup.
// Aman dipanggil lebih dari sekali — hanya panggilan pertama yang beroutput.
func (w *AnthropicSSEWriter) Finish() string {
	if w.stopped {
		return ""
	}
	w.stopped = true
	var sb strings.Builder
	if !w.endSent {
		for _, f := range w.writeEnd(Event{Type: EventMessageEnd, Delta: &StreamDelta{FinishReason: FinishStop}}) {
			sb.WriteString(f)
		}
	}
	sb.WriteString(frame("message_stop", map[string]any{"type": "message_stop"}))
	return sb.String()
}

// Tokens mengembalikan pemakaian terakhir yang diketahui writer.
func (w *AnthropicSSEWriter) Tokens() (int, int) { return w.inputTok, w.outputTok }
