package translate

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseOpenAIRequestSimpleText(t *testing.T) {
	body := []byte(`{
		"model": "oa/gpt-5.4",
		"messages": [
			{"role": "system", "content": "Kamu asisten."},
			{"role": "user", "content": "Halo"}
		],
		"temperature": 0.5,
		"max_tokens": 128
	}`)
	req, err := ParseOpenAIRequest(body)
	if err != nil {
		t.Fatalf("ParseOpenAIRequest: %v", err)
	}
	if req.Model != "oa/gpt-5.4" || len(req.Messages) != 2 {
		t.Fatalf("req = %+v", req)
	}
	if req.Messages[0].TextContent() != "Kamu asisten." {
		t.Errorf("system = %q", req.Messages[0].TextContent())
	}
	if req.Temperature == nil || *req.Temperature != 0.5 {
		t.Error("temperature")
	}
	if req.MaxTokens == nil || *req.MaxTokens != 128 {
		t.Error("max_tokens")
	}
}

func TestParseOpenAIRequestContentArrayAndNull(t *testing.T) {
	body := []byte(`{
		"model": "m",
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "Apa ini?"},
				{"type": "image_url", "image_url": {"url": "data:image/png;base64,AAAA", "detail": "low"}}
			]},
			{"role": "assistant", "content": null},
			{"role": "assistant", "content": [{"type":"refusal","text":"tidak"}]}
		]
	}`)
	req, err := ParseOpenAIRequest(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m0 := req.Messages[0]
	if len(m0.Content) != 2 || m0.Content[0].Text != "Apa ini?" {
		t.Fatalf("parts = %+v", m0.Content)
	}
	if m0.Content[1].ImageURL != "data:image/png;base64,AAAA" || m0.Content[1].Detail != "low" {
		t.Errorf("image = %+v", m0.Content[1])
	}
	if req.Messages[1].Content != nil {
		t.Errorf("null content harus nil, dapat %+v", req.Messages[1].Content)
	}
	if req.Messages[2].TextContent() != "tidak" {
		t.Errorf("refusal harus jadi teks: %q", req.Messages[2].TextContent())
	}
}

func TestParseOpenAIRequestStopVariants(t *testing.T) {
	req, err := ParseOpenAIRequest([]byte(`{"model":"m","messages":[],"stop":"END"}`))
	if err != nil || len(req.Stop) != 1 || req.Stop[0] != "END" {
		t.Fatalf("stop string: %v %v", req.Stop, err)
	}
	req, err = ParseOpenAIRequest([]byte(`{"model":"m","messages":[],"stop":["a","b"]}`))
	if err != nil || len(req.Stop) != 2 {
		t.Fatalf("stop array: %v %v", req.Stop, err)
	}
}

func TestParseOpenAIRequestTools(t *testing.T) {
	body := []byte(`{
		"model": "m",
		"messages": [{"role":"user","content":"Cuaca Jakarta?"}],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Ambil cuaca",
				"parameters": {"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}
			}
		}],
		"tool_choice": "auto",
		"stream_options": {"include_usage": true}
	}`)
	req, err := ParseOpenAIRequest(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "get_weather" {
		t.Fatalf("tools = %+v", req.Tools)
	}
	if req.ToolChoice == nil || req.ToolChoice.Mode != ToolChoiceAuto {
		t.Fatalf("tool_choice = %+v", req.ToolChoice)
	}
	if !req.StreamUsage {
		t.Error("stream_usage harus true")
	}
	if !json.Valid(req.Tools[0].Parameters) {
		t.Error("parameters harus JSON valid")
	}
}

func TestParseOpenAIToolChoiceVariants(t *testing.T) {
	for raw, want := range map[string]ToolChoiceMode{
		`"auto"`:     ToolChoiceAuto,
		`"none"`:     ToolChoiceNone,
		`"required"`: ToolChoiceRequired,
		`{"type":"function","function":{"name":"fn"}}`: ToolChoiceNamed,
	} {
		tc, err := parseOpenAIToolChoice(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if tc.Mode != want {
			t.Errorf("%s → %s, mau %s", raw, tc.Mode, want)
		}
		if want == ToolChoiceNamed && tc.Name != "fn" {
			t.Errorf("named name = %q", tc.Name)
		}
	}
}

func TestParseOpenAIRequestMaxCompletionTokens(t *testing.T) {
	req, err := ParseOpenAIRequest([]byte(`{"model":"m","messages":[],"max_completion_tokens":77}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 77 {
		t.Error("max_completion_tokens harus dipetakan ke MaxTokens")
	}
}

func TestParseOpenAIRequestInvalid(t *testing.T) {
	if _, err := ParseOpenAIRequest([]byte(`{bukan json`)); err == nil {
		t.Fatal("harus error")
	}
}

func TestRenderOpenAIRequestRoundTrip(t *testing.T) {
	temp := 0.3
	mt := 100
	req := &ChatRequest{
		Model: "local/qwen3-8b",
		Messages: []Message{
			{Role: RoleSystem, Content: []Part{{Type: PartText, Text: "s"}}},
			{Role: RoleUser, Content: []Part{
				{Type: PartText, Text: "lihat gambar"},
				{Type: PartImage, ImageURL: "https://x/y.png"},
			}},
			{Role: RoleAssistant, Content: []Part{{Type: PartText, Text: "ok"}},
				ToolCalls: []ToolCall{{ID: "call_1", Name: "fn", Arguments: `{"a":1}`}}},
			{Role: RoleTool, ToolCallID: "call_1", Content: []Part{{Type: PartText, Text: `{"res":2}`}}},
		},
		Tools:       []Tool{{Name: "fn", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}},
		ToolChoice:  &ToolChoice{Mode: ToolChoiceNamed, Name: "fn"},
		Temperature: &temp,
		MaxTokens:   &mt,
		Stop:        []string{"STOP"},
		Stream:      true,
		StreamUsage: true,
	}
	out, err := RenderOpenAIRequest(req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// re-parse → harus setara
	back, err := ParseOpenAIRequest(out)
	if err != nil {
		t.Fatalf("re-parse: %v (%s)", err, out)
	}
	if back.Model != req.Model || len(back.Messages) != 4 {
		t.Fatalf("back = %+v", back)
	}
	if back.Messages[1].Content[1].ImageURL != "https://x/y.png" {
		t.Errorf("image lost: %+v", back.Messages[1].Content)
	}
	if len(back.Messages[2].ToolCalls) != 1 || back.Messages[2].ToolCalls[0].Name != "fn" {
		t.Errorf("tool_calls lost: %+v", back.Messages[2].ToolCalls)
	}
	if back.Messages[3].Role != RoleTool || back.Messages[3].ToolCallID != "call_1" {
		t.Errorf("tool message lost: %+v", back.Messages[3])
	}
	if len(back.Tools) != 1 || back.Tools[0].Name != "fn" {
		t.Errorf("tools lost: %+v", back.Tools)
	}
	if back.ToolChoice == nil || back.ToolChoice.Name != "fn" {
		t.Errorf("tool_choice lost: %+v", back.ToolChoice)
	}
	// content teks murni harus dirender sebagai string (bukan array)
	var w map[string]any
	json.Unmarshal(out, &w)
	msgs := w["messages"].([]any)
	m0 := msgs[0].(map[string]any)
	if _, isArr := m0["content"].([]any); isArr {
		t.Error("content teks murni harus string")
	}
	// image harus array
	m1 := msgs[1].(map[string]any)
	if _, isArr := m1["content"].([]any); !isArr {
		t.Error("content dengan gambar harus array")
	}
	if _, ok := w["stream_options"].(map[string]any)["include_usage"]; !ok {
		t.Error("stream_options.include_usage hilang")
	}
}

func TestParseOpenAIResponseBasic(t *testing.T) {
	body := []byte(`{
		"id": "chatcmpl-123", "object": "chat.completion", "created": 1700000000,
		"model": "gpt-x",
		"choices": [{"index":0,"message":{"role":"assistant","content":"Halo!"},"finish_reason":"stop"}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
	}`)
	resp, err := ParseOpenAIResponse(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Content != "Halo!" || resp.FinishReason != FinishStop {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Usage.PromptTokens != 10 || resp.Usage.CompletionTokens != 5 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if resp.IsEmpty() {
		t.Error("respons berisi tidak boleh IsEmpty")
	}
}

func TestParseOpenAIResponseToolCalls(t *testing.T) {
	body := []byte(`{
		"id": "chatcmpl-9", "model": "m",
		"choices": [{"index":0,"message":{"role":"assistant","content":null,
			"tool_calls":[{"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Jakarta\"}"}}]},
			"finish_reason":"tool_calls"}],
		"usage": {"prompt_tokens": 7, "completion_tokens": 9}
	}`)
	resp, err := ParseOpenAIResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "get_weather" {
		t.Fatalf("toolcalls = %+v", resp.ToolCalls)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("finish = %s", resp.FinishReason)
	}
}

func TestParseOpenAIResponseUpstreamError(t *testing.T) {
	resp, err := ParseOpenAIResponse([]byte(`{"error":{"message":"kurang kuota","type":"insufficient_quota","code":"429"}}`))
	if err == nil {
		t.Fatal("harus error")
	}
	ue, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("tipe error = %T", err)
	}
	if ue.Message != "kurang kuota" || ue.Code != "429" {
		t.Fatalf("ue = %+v", ue)
	}
	_ = resp
}

func TestRenderOpenAIResponseShape(t *testing.T) {
	resp := &ChatResponse{
		ID: "chatcmpl-x", Model: "an/claude-sonnet-4-6", Content: "jawaban",
		FinishReason: FinishStop,
		Usage:        Usage{PromptTokens: 11, CompletionTokens: 7},
	}
	out, err := RenderOpenAIResponse(resp)
	if err != nil {
		t.Fatal(err)
	}
	var w map[string]any
	if err := json.Unmarshal(out, &w); err != nil {
		t.Fatal(err)
	}
	if w["object"] != "chat.completion" {
		t.Errorf("object = %v", w["object"])
	}
	choices := w["choices"].([]any)
	c0 := choices[0].(map[string]any)
	if c0["finish_reason"] != "stop" {
		t.Errorf("finish = %v", c0["finish_reason"])
	}
	usage := w["usage"].(map[string]any)
	if usage["total_tokens"].(float64) != 18 {
		t.Errorf("total = %v", usage["total_tokens"])
	}
}

func TestOpenAISSEParseChunk(t *testing.T) {
	events, err := ParseOpenAISSEChunk([]byte(`{"id":"c1","object":"chat.completion.chunk","model":"m",
		"choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != EventDelta || events[0].Delta.Role != "assistant" {
		t.Fatalf("events = %+v", events)
	}

	events, err = ParseOpenAISSEChunk([]byte(`{"choices":[{"index":0,"delta":{"content":"Ha"},"finish_reason":null}]}`))
	if err != nil || len(events) != 1 || events[0].Delta.Text != "Ha" {
		t.Fatalf("text delta: %+v %v", events, err)
	}

	// tool call incremental
	events, err = ParseOpenAISSEChunk([]byte(`{"choices":[{"index":0,"delta":{"tool_calls":[
		{"index":0,"id":"call_1","type":"function","function":{"name":"f","arguments":""}},
		{"index":0,"function":{"arguments":"{\"x\":"}},
		{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":null}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || len(events[0].Delta.ToolCalls) != 3 {
		t.Fatalf("tool deltas: %+v", events)
	}

	// finish + usage chunk kosong
	events, err = ParseOpenAISSEChunk([]byte(`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`))
	if err != nil || len(events) != 1 || events[0].Type != EventMessageEnd {
		t.Fatalf("end: %+v %v", events, err)
	}
	events, err = ParseOpenAISSEChunk([]byte(`{"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4}}`))
	if err != nil || len(events) != 1 || events[0].Type != EventUsage {
		t.Fatalf("usage: %+v %v", events, err)
	}

	// [DONE] → tidak ada event
	events, err = ParseOpenAISSEChunk([]byte("[DONE]"))
	if err != nil || events != nil {
		t.Fatalf("done: %v %v", events, err)
	}

	// error event
	events, err = ParseOpenAISSEChunk([]byte(`{"error":{"message":"boom","type":"server_error","code":500}}`))
	if err != nil || len(events) != 1 || events[0].Type != EventError || events[0].Error.Code != "500" {
		t.Fatalf("error: %+v %v", events, err)
	}
}

func TestOpenAISSERenderChunk(t *testing.T) {
	begin := RenderOpenAISSEChunk(Event{Type: EventMessageBegin}, "id1", "m1")
	if !strings.Contains(begin, `"role":"assistant"`) || !strings.Contains(begin, `"model":"m1"`) {
		t.Fatalf("begin = %s", begin)
	}
	delta := RenderOpenAISSEChunk(Event{Type: EventDelta, Delta: &StreamDelta{Text: "Hai"}}, "id1", "m1")
	if !strings.Contains(delta, `"content":"Hai"`) {
		t.Fatalf("delta = %s", delta)
	}
	// delta kosong → tidak render
	if empty := RenderOpenAISSEChunk(Event{Type: EventDelta, Delta: &StreamDelta{}}, "id1", "m1"); empty != "" {
		t.Errorf("delta kosong harus '': %s", empty)
	}
	end := RenderOpenAISSEChunk(Event{Type: EventMessageEnd, Delta: &StreamDelta{FinishReason: FinishToolCalls}}, "id1", "m1")
	if !strings.Contains(end, `"finish_reason":"tool_calls"`) {
		t.Fatalf("end = %s", end)
	}
	usage := RenderOpenAISSEChunk(Event{Type: EventUsage, Usage: &Usage{PromptTokens: 2, CompletionTokens: 3}}, "id1", "m1")
	if !strings.Contains(usage, `"total_tokens":5`) || !strings.Contains(usage, `"choices":[]`) {
		t.Fatalf("usage = %s", usage)
	}
	// tool call delta render
	tcd := RenderOpenAISSEChunk(Event{Type: EventDelta, Delta: &StreamDelta{ToolCalls: []ToolCallDelta{{
		Index: 0, ID: "call_1", Name: "fn", Arguments: "{\"a\"",
	}}}}, "id1", "m1")
	if !strings.Contains(tcd, `"id":"call_1"`) || !strings.Contains(tcd, `"index":0`) {
		t.Fatalf("tool delta = %s", tcd)
	}
}

func TestRenderOpenAIErrorFormat(t *testing.T) {
	out := RenderOpenAIError(401, "invalid_api_key", "key salah")
	var w struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out, &w); err != nil {
		t.Fatal(err)
	}
	if w.Error.Code != "invalid_api_key" || w.Error.Message != "key salah" || w.Error.Type == "" {
		t.Fatalf("error shape = %+v", w.Error)
	}
}

func TestEstimateTokens(t *testing.T) {
	if EstimateTokens("") != 0 {
		t.Error("empty harus 0")
	}
	en := EstimateTokens(strings.Repeat("abcdefgh", 10)) // 80 char latin ≈ 20
	if en < 15 || en > 25 {
		t.Errorf("latin estimate = %d", en)
	}
	idn := EstimateTokens(strings.Repeat("안녕하세요", 10)) // CJK: 50 rune ≈ 33
	if idn < 20 {
		t.Errorf("cjk estimate = %d", idn)
	}
	msgs := []Message{
		{Role: RoleUser, Content: []Part{{Type: PartText, Text: strings.Repeat("x", 40)}}},
	}
	if EstimateMessages(msgs) < 10 {
		t.Error("estimate messages terlalu kecil")
	}
}

func TestUpstreamErrorRetryable(t *testing.T) {
	for code, want := range map[int]bool{
		429: true, 500: true, 503: true, 402: true, 408: true, 0: true,
		400: false, 401: false, 404: false, 422: false,
	} {
		e := &UpstreamError{StatusCode: code}
		if e.Retryable() != want {
			t.Errorf("status %d retryable = %v, mau %v", code, e.Retryable(), want)
		}
	}
}

func TestChatResponseIsEmpty(t *testing.T) {
	if !(&ChatResponse{}).IsEmpty() {
		t.Error("kosong harus true")
	}
	if !(&ChatResponse{Content: "   "}).IsEmpty() {
		t.Error("whitespace harus dianggap kosong")
	}
	if (&ChatResponse{Content: "hi"}).IsEmpty() {
		t.Error("isi tidak boleh kosong")
	}
	if (&ChatResponse{ToolCalls: []ToolCall{{Name: "f"}}}).IsEmpty() {
		t.Error("tool call bukan kosong")
	}
}
