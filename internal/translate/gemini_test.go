package translate

import (
	"encoding/json"
	"strings"
	"testing"
)

func geminiTestRequest() *ChatRequest {
	mt := 256
	temp := 0.2
	return &ChatRequest{
		Model:       "gm/gemini-2.5-flash",
		MaxTokens:   &mt,
		Temperature: &temp,
		Messages: []Message{
			{Role: RoleSystem, Content: []Part{{Type: PartText, Text: "Jawab singkat."}}},
			{Role: RoleUser, Content: []Part{{Type: PartText, Text: "Apa itu Go?"}}},
			{Role: RoleAssistant, Content: []Part{{Type: PartText, Text: "Bahasa pemrograman."}}},
			{Role: RoleUser, Content: []Part{{Type: PartText, Text: "Contoh kode?"}}},
		},
	}
}

func respAsJSON(v any) []byte {
	switch t := v.(type) {
	case string:
		return []byte(t)
	default:
		b, _ := json.Marshal(t)
		return b
	}
}

func TestRenderGeminiRequestBasics(t *testing.T) {
	out, err := RenderGeminiRequest(geminiTestRequest())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var w map[string]any
	if err := json.Unmarshal(out, &w); err != nil {
		t.Fatal(err)
	}
	sys, ok := w["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatalf("systemInstruction hilang: %s", out)
	}
	sysParts := sys["parts"].([]any)
	if sysParts[0].(map[string]any)["text"] != "Jawab singkat." {
		t.Errorf("system text = %v", sysParts[0])
	}
	contents := w["contents"].([]any)
	if len(contents) != 3 {
		t.Fatalf("contents = %d", len(contents))
	}
	if contents[0].(map[string]any)["role"] != "user" {
		t.Error("role pertama harus user")
	}
	if contents[1].(map[string]any)["role"] != "model" {
		t.Error("assistant harus jadi model")
	}
	gc := w["generationConfig"].(map[string]any)
	if gc["maxOutputTokens"].(float64) != 256 {
		t.Errorf("maxOutputTokens = %v", gc["maxOutputTokens"])
	}
	if gc["temperature"].(float64) != 0.2 {
		t.Errorf("temperature = %v", gc["temperature"])
	}
}

func TestRenderGeminiRequestImageAndStop(t *testing.T) {
	req := &ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: []Part{
			{Type: PartText, Text: "gambar ini?"},
			{Type: PartImage, ImageURL: "data:image/webp;base64,SVRC"},
		}}},
		Stop: []string{"STOP"},
	}
	out, err := RenderGeminiRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var w map[string]any
	json.Unmarshal(out, &w)
	contents := w["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	img := parts[1].(map[string]any)["inlineData"].(map[string]any)
	if img["mimeType"] != "image/webp" || img["data"] != "SVRC" {
		t.Fatalf("inlineData = %v", img)
	}
	gc := w["generationConfig"].(map[string]any)
	stops := gc["stopSequences"].([]any)
	if stops[0] != "STOP" {
		t.Errorf("stop = %v", stops)
	}
}

func TestRenderGeminiRequestResponseFormatJSON(t *testing.T) {
	req := &ChatRequest{
		Messages:   []Message{{Role: RoleUser, Content: []Part{{Type: PartText, Text: "json"}}}},
		RespFormat: &ResponseFormat{Type: "json_object"},
	}
	out, _ := RenderGeminiRequest(req)
	if !strings.Contains(string(out), `"responseMimeType":"application/json"`) {
		t.Errorf("responseMimeType hilang: %s", out)
	}
}

func TestGeminiURL(t *testing.T) {
	if got := GeminiURL("https://generativelanguage.googleapis.com/v1beta", "gemini-2.5-flash", false); got !=
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent" {
		t.Errorf("url = %s", got)
	}
	if got := GeminiURL("https://x/v1beta/", "m", true); got != "https://x/v1beta/models/m:streamGenerateContent?alt=sse" {
		t.Errorf("url stream = %s", got)
	}
}

func TestRenderGeminiRequestToolLoop(t *testing.T) {
	req := &ChatRequest{
		Model: "m",
		Messages: []Message{
			{Role: RoleUser, Content: []Part{{Type: PartText, Text: "cuaca?"}}},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_fn", Name: "get_weather", Arguments: `{"city":"Bogor"}`}}},
			{Role: RoleTool, ToolCallID: "call_fn", Content: []Part{{Type: PartText, Text: `{"temp":25}`}}},
		},
		Tools:      []Tool{{Name: "get_weather", Description: "cuaca", Parameters: json.RawMessage(`{"type":"object"}`)}},
		ToolChoice: &ToolChoice{Mode: ToolChoiceAuto},
	}
	out, err := RenderGeminiRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var w map[string]any
	json.Unmarshal(out, &w)
	contents := w["contents"].([]any)
	if len(contents) != 3 {
		t.Fatalf("contents = %d: %s", len(contents), out)
	}
	modelMsg := contents[1].(map[string]any)
	fc := modelMsg["parts"].([]any)[0].(map[string]any)["functionCall"].(map[string]any)
	if fc["name"] != "get_weather" {
		t.Errorf("functionCall = %v", fc)
	}
	respMsg := contents[2].(map[string]any)
	fr := respMsg["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
	if fr["name"] != "get_weather" {
		t.Errorf("functionResponse name = %v", fr["name"])
	}
	var respBody map[string]any
	json.Unmarshal(respAsJSON(fr["response"]), &respBody)
	if respBody["temp"].(float64) != 25 {
		t.Errorf("response = %v", respBody)
	}
	tools := w["tools"].([]any)
	if tools[0].(map[string]any)["functionDeclarations"] == nil {
		t.Error("functionDeclarations hilang")
	}
	if w["toolConfig"].(map[string]any)["functionCallingConfig"].(map[string]any)["mode"] != "AUTO" {
		t.Errorf("toolConfig = %v", w["toolConfig"])
	}
}

func TestRenderGeminiToolResultPlainText(t *testing.T) {
	req := &ChatRequest{
		Messages: []Message{
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "fn", Arguments: `{}`}}},
			{Role: RoleTool, ToolCallID: "c1", Content: []Part{{Type: PartText, Text: "teks biasa"}}},
		},
	}
	out, _ := RenderGeminiRequest(req)
	var w map[string]any
	json.Unmarshal(out, &w)
	fr := w["contents"].([]any)[1].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
	var resp map[string]any
	json.Unmarshal(respAsJSON(fr["response"]), &resp)
	if resp["result"] != "teks biasa" {
		t.Errorf("response = %v", resp)
	}
}

func TestParseGeminiResponseText(t *testing.T) {
	body := []byte(`{
		"candidates": [{
			"content": {"role": "model", "parts": [{"text": "Halo juga"}]},
			"finishReason": "STOP"
		}],
		"usageMetadata": {"promptTokenCount": 12, "candidatesTokenCount": 4}
	}`)
	resp, err := ParseGeminiResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Halo juga" || resp.FinishReason != FinishStop {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Usage.PromptTokens != 12 || resp.Usage.CompletionTokens != 4 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestParseGeminiResponseToolCallAndSafety(t *testing.T) {
	body := []byte(`{
		"candidates": [{
			"content": {"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Depok"}}}]},
			"finishReason": "STOP"
		}],
		"usageMetadata": {"promptTokenCount": 8, "candidatesTokenCount": 6}
	}`)
	resp, err := ParseGeminiResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "get_weather" {
		t.Fatalf("toolcalls = %+v", resp.ToolCalls)
	}
	var args map[string]any
	json.Unmarshal([]byte(resp.ToolCalls[0].Arguments), &args)
	if args["city"] != "Depok" {
		t.Errorf("args = %v", args)
	}

	// diblok safety
	_, err = ParseGeminiResponse([]byte(`{"promptFeedback":{"blockReason":"SAFETY"}}`))
	if err == nil {
		t.Fatal("blockReason harus error")
	}
	// error body
	_, err = ParseGeminiResponse([]byte(`{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}`))
	if err == nil {
		t.Fatal("error body harus error")
	}
	ue := err.(*UpstreamError)
	if ue.StatusCode != 429 || !ue.Retryable() {
		t.Errorf("ue = %+v", ue)
	}
	// MAX_TOKENS
	resp, _ = ParseGeminiResponse([]byte(`{"candidates":[{"content":{"parts":[{"text":"potong"}]},"finishReason":"MAX_TOKENS"}]}`))
	if resp.FinishReason != FinishLength {
		t.Errorf("finish = %s", resp.FinishReason)
	}
}

func TestParseGeminiSSEChunk(t *testing.T) {
	evs, err := ParseGeminiSSEChunk([]byte(`{"candidates":[{"content":{"parts":[{"text":"Hi"}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Type != EventDelta || evs[0].Delta.Text != "Hi" {
		t.Fatalf("evs = %+v", evs)
	}
	evs, err = ParseGeminiSSEChunk([]byte(`{"candidates":[{"content":{"parts":[{"text":"selesai"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	// delta + end + usage
	if len(evs) != 3 || evs[1].Type != EventMessageEnd || evs[2].Type != EventUsage {
		t.Fatalf("evs = %+v", evs)
	}
	evs, err = ParseGeminiSSEChunk([]byte(`{"error":{"code":500,"message":"internal","status":"INTERNAL"}}`))
	if err != nil || len(evs) != 1 || evs[0].Type != EventError || evs[0].Error.StatusCode != 500 {
		t.Fatalf("evs=%+v err=%v", evs, err)
	}
}
