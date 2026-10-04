package translate

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ---------- Request (internal → Gemini) ----------

type wireGeminiRequest struct {
	Contents          []wireGeminiContent   `json:"contents"`
	SystemInstruction *wireGeminiContent    `json:"systemInstruction,omitempty"`
	GenerationConfig  *wireGeminiGenConfig  `json:"generationConfig,omitempty"`
	Tools             []wireGeminiTool      `json:"tools,omitempty"`
	ToolConfig        *wireGeminiToolConfig `json:"toolConfig,omitempty"`
	SafetySettings    []any                 `json:"safetySettings,omitempty"`
}

type wireGeminiContent struct {
	Role  string           `json:"role,omitempty"` // "user" | "model"
	Parts []wireGeminiPart `json:"parts"`
}

type wireGeminiPart struct {
	Text             string                `json:"text,omitempty"`
	InlineData       *wireGeminiInlineData `json:"inlineData,omitempty"`
	FunctionCall     *wireGeminiFuncCall   `json:"functionCall,omitempty"`
	FunctionResponse *wireGeminiFuncResp   `json:"functionResponse,omitempty"`
}

type wireGeminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64
}

type wireGeminiFuncCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type wireGeminiFuncResp struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type wireGeminiGenConfig struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"topP,omitempty"`
	MaxOutputTokens  *int     `json:"maxOutputTokens,omitempty"`
	StopSequences    []string `json:"stopSequences,omitempty"`
	ResponseMimeType string   `json:"responseMimeType,omitempty"`
}

type wireGeminiTool struct {
	FunctionDeclarations []wireGeminiFuncDecl `json:"functionDeclarations"`
}

type wireGeminiFuncDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type wireGeminiToolConfig struct {
	FunctionCallingConfig struct {
		Mode string `json:"mode"` // AUTO | ANY | NONE
	} `json:"functionCallingConfig"`
}

// RenderGeminiRequest mengubah ChatRequest menjadi body JSON generateContent.
func RenderGeminiRequest(req *ChatRequest) ([]byte, error) {
	w := wireGeminiRequest{}
	var systemTexts []string
	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			if t := m.TextContent(); t != "" {
				systemTexts = append(systemTexts, t)
			}
		case RoleUser:
			c := wireGeminiContent{Role: "user"}
			for _, p := range m.Content {
				if p.Type == PartImage {
					if strings.HasPrefix(p.ImageURL, "data:") {
						media, data := parseDataURL(p.ImageURL)
						c.Parts = append(c.Parts, wireGeminiPart{InlineData: &wireGeminiInlineData{MimeType: media, Data: data}})
					}
					// URL http(s) gambar tidak didukung Gemini inline → dilewati
				} else if p.Text != "" {
					c.Parts = append(c.Parts, wireGeminiPart{Text: p.Text})
				}
			}
			if len(c.Parts) > 0 {
				w.Contents = append(w.Contents, c)
			}
		case RoleAssistant:
			c := wireGeminiContent{Role: "model"}
			if t := m.TextContent(); t != "" {
				c.Parts = append(c.Parts, wireGeminiPart{Text: t})
			}
			for _, tc := range m.ToolCalls {
				args := json.RawMessage(tc.Arguments)
				if !json.Valid(args) {
					args = json.RawMessage(`{}`)
				}
				c.Parts = append(c.Parts, wireGeminiPart{FunctionCall: &wireGeminiFuncCall{Name: tc.Name, Args: args}})
			}
			if len(c.Parts) > 0 {
				w.Contents = append(w.Contents, c)
			}
		case RoleTool:
			// hasil tool → functionResponse milik role "user"
			var respParsed json.RawMessage
			trimmed := strings.TrimSpace(m.TextContent())
			if trimmed != "" && json.Valid([]byte(trimmed)) && (trimmed[0] == '{' || trimmed[0] == '[') {
				respParsed = json.RawMessage(trimmed)
			} else {
				respParsed = json.RawMessage(marshalString(trimmed))
			}
			// Gemini mengirim response sebagai objek: {"result": ...} bila bukan objek
			if len(respParsed) > 0 && respParsed[0] == '"' {
				respParsed = json.RawMessage(`{"result":` + string(respParsed) + `}`)
			}
			name := toolNameForCallID(req, m.ToolCallID)
			w.Contents = append(w.Contents, wireGeminiContent{Role: "user", Parts: []wireGeminiPart{{
				FunctionResponse: &wireGeminiFuncResp{Name: name, Response: respParsed},
			}}})
		}
	}
	if len(systemTexts) > 0 {
		w.SystemInstruction = &wireGeminiContent{Parts: []wireGeminiPart{{Text: strings.Join(systemTexts, "\n")}}}
	}
	gc := &wireGeminiGenConfig{
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		StopSequences: req.Stop,
	}
	if req.MaxTokens != nil {
		gc.MaxOutputTokens = req.MaxTokens
	}
	if req.RespFormat != nil && req.RespFormat.Type == "json_object" {
		gc.ResponseMimeType = "application/json"
	}
	w.GenerationConfig = gc
	for _, tl := range req.Tools {
		params := tl.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object"}`)
		}
		w.Tools = append(w.Tools, wireGeminiTool{FunctionDeclarations: []wireGeminiFuncDecl{{
			Name: tl.Name, Description: tl.Description, Parameters: params,
		}}})
	}
	if req.ToolChoice != nil {
		tc := &wireGeminiToolConfig{}
		switch req.ToolChoice.Mode {
		case ToolChoiceAuto:
			tc.FunctionCallingConfig.Mode = "AUTO"
		case ToolChoiceRequired, ToolChoiceNamed:
			tc.FunctionCallingConfig.Mode = "ANY"
		case ToolChoiceNone:
			tc.FunctionCallingConfig.Mode = "NONE"
		}
		w.ToolConfig = tc
	}
	return json.Marshal(w)
}

// toolNameForCallID mencari nama function berdasarkan ID tool call.
func toolNameForCallID(req *ChatRequest, callID string) string {
	if callID == "" {
		return "unknown"
	}
	for _, m := range req.Messages {
		for _, tc := range m.ToolCalls {
			if tc.ID == callID {
				return tc.Name
			}
		}
	}
	return "unknown"
}

// GeminiURL membangun URL endpoint Gemini sesuai stream/bukan.
func GeminiURL(base, model string, stream bool) string {
	base = strings.TrimRight(base, "/")
	method := "generateContent"
	if stream {
		method = "streamGenerateContent?alt=sse"
	}
	return fmt.Sprintf("%s/models/%s:%s", base, model, method)
}

// ---------- Respons (Gemini → internal) ----------

type wireGeminiResponse struct {
	Candidates []struct {
		Content struct {
			Role  string           `json:"role"`
			Parts []wireGeminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason,omitempty"`
	} `json:"promptFeedback"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// finishGeminiToInternal memetakan finishReason Gemini → internal.
func finishGeminiToInternal(s string) string {
	switch s {
	case "STOP":
		return FinishStop
	case "MAX_TOKENS":
		return FinishLength
	case "SAFETY", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return FinishContentFilter
	case "MALFORMED_FUNCTION_CALL":
		return FinishStop
	default:
		return FinishStop
	}
}

// ParseGeminiResponse mengubah respons JSON Gemini non-stream.
func ParseGeminiResponse(body []byte) (*ChatResponse, error) {
	var w wireGeminiResponse
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("respons upstream bukan JSON Gemini sah: %w", err)
	}
	if w.Error != nil {
		return nil, &UpstreamError{StatusCode: w.Error.Code, Type: w.Error.Status, Message: w.Error.Message}
	}
	if w.PromptFeedback.BlockReason != "" {
		return nil, &UpstreamError{StatusCode: 400, Type: "content_filter",
			Message: "permintaan diblokir filter keamanan provider: " + w.PromptFeedback.BlockReason}
	}
	resp := &ChatResponse{}
	var b strings.Builder
	if len(w.Candidates) > 0 {
		c := w.Candidates[0]
		for _, p := range c.Content.Parts {
			switch {
			case p.Text != "":
				b.WriteString(p.Text)
			case p.FunctionCall != nil:
				args := string(p.FunctionCall.Args)
				if args == "" || !json.Valid([]byte(args)) {
					args = "{}"
				}
				resp.ToolCalls = append(resp.ToolCalls, ToolCall{
					ID:        "call_" + p.FunctionCall.Name,
					Name:      p.FunctionCall.Name,
					Arguments: args,
				})
			}
		}
		resp.FinishReason = finishGeminiToInternal(c.FinishReason)
	}
	resp.Content = b.String()
	resp.Usage = Usage{PromptTokens: w.UsageMetadata.PromptTokenCount, CompletionTokens: w.UsageMetadata.CandidatesTokenCount}
	return resp, nil
}

// ParseGeminiSSEChunk mengubah satu payload `data:` dari
// streamGenerateContent?alt=sse menjadi event internal.
func ParseGeminiSSEChunk(data []byte) ([]Event, error) {
	var w wireGeminiResponse
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("chunk SSE Gemini tidak sah: %w", err)
	}
	if w.Error != nil {
		return []Event{{Type: EventError, Error: &UpstreamError{
			StatusCode: w.Error.Code, Type: w.Error.Status, Message: w.Error.Message,
		}}}, nil
	}
	resp, err := ParseGeminiResponse(data)
	if err != nil {
		return nil, err
	}
	var events []Event
	if resp.Content != "" {
		events = append(events, Event{Type: EventDelta, Delta: &StreamDelta{Text: resp.Content}})
	}
	for _, tc := range resp.ToolCalls {
		events = append(events, Event{Type: EventDelta, Delta: &StreamDelta{
			ToolCalls: []ToolCallDelta{{Index: 0, ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments}},
		}})
	}
	if len(w.Candidates) > 0 && w.Candidates[0].FinishReason != "" {
		events = append(events, Event{Type: EventMessageEnd, Delta: &StreamDelta{FinishReason: resp.FinishReason}})
	}
	if resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
		events = append(events, Event{Type: EventUsage, Usage: &Usage{
			PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens,
		}})
	}
	return events, nil
}

// RenderGeminiError menormalkan pesan error Gemini.
func RenderGeminiError(status int, message string) []byte {
	b, _ := json.Marshal(map[string]any{"error": map[string]any{
		"code": status, "message": message, "status": "ERROR",
	}})
	return b
}
