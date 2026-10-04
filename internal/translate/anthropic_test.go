package translate

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAnthropicRequestBasic(t *testing.T) {
	body := []byte(`{
		"model": "an/claude-sonnet-4-6",
		"max_tokens": 1024,
		"system": "Kamu asisten singkat.",
		"messages": [
			{"role": "user", "content": "Halo"},
			{"role": "assistant", "content": "Hai!"},
			{"role": "user", "content": [
				{"type": "text", "text": "Lihat gambar"},
				{"type": "image", "source": {"type": "base64", "media_type": "image/jpeg", "data": "QQ=="}}
			]}
		],
		"temperature": 0.7,
		"stop_sequences": ["SELESAI"]
	}`)
	req, err := ParseAnthropicRequest(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if req.Model != "an/claude-sonnet-4-6" {
		t.Errorf("model = %s", req.Model)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 1024 {
		t.Error("max_tokens")
	}
	if len(req.Messages) != 4 {
		t.Fatalf("messages = %d", len(req.Messages))
	}
	if req.Messages[0].Role != RoleSystem || req.Messages[0].TextContent() != "Kamu asisten singkat." {
		t.Errorf("system = %+v", req.Messages[0])
	}
	if req.Messages[3].Content[1].ImageURL != "data:image/jpeg;base64,QQ==" {
		t.Errorf("image = %+v", req.Messages[3].Content[1])
	}
	if req.Stop[0] != "SELESAI" {
		t.Errorf("stop = %v", req.Stop)
	}
}

func TestParseAnthropicRequestRequiresMaxTokens(t *testing.T) {
	if _, err := ParseAnthropicRequest([]byte(`{"model":"m","messages":[]}`)); err == nil {
		t.Fatal("max_tokens wajib")
	}
	if _, err := ParseAnthropicRequest([]byte(`{"messages":[]}`)); err == nil {
		t.Fatal("model wajib")
	}
}

func TestParseAnthropicRequestToolLoop(t *testing.T) {
	body := []byte(`{
		"model": "m", "max_tokens": 100,
		"messages": [
			{"role": "user", "content": "Cuaca di Bandung?"},
			{"role": "assistant", "content": [
				{"type": "text", "text": "Saya cek dulu."},
				{"type": "tool_use", "id": "toolu_1", "name": "get_weather", "input": {"city": "Bandung"}}
			]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "toolu_1", "content": "24C cerah"}
			]}
		],
		"tools": [{"name": "get_weather", "description": "Cuaca", "input_schema": {"type":"object"}}],
		"tool_choice": {"type": "auto"}
	}`)
	req, err := ParseAnthropicRequest(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(req.Messages) != 4 {
		t.Fatalf("messages = %d: %+v", len(req.Messages), req.Messages)
	}
	// teks dan tool_use dalam satu wire message → dua pesan internal berurutan
	asstText := req.Messages[1]
	if asstText.Role != RoleAssistant || asstText.TextContent() != "Saya cek dulu." {
		t.Errorf("assistant text = %+v", asstText)
	}
	asst := req.Messages[2]
	if len(asst.ToolCalls) != 1 || asst.ToolCalls[0].ID != "toolu_1" || asst.ToolCalls[0].Name != "get_weather" {
		t.Errorf("tool_calls = %+v", asst.ToolCalls)
	}
	if asst.ToolCalls[0].Arguments == "" || !json.Valid([]byte(asst.ToolCalls[0].Arguments)) {
		t.Errorf("arguments tidak valid: %s", asst.ToolCalls[0].Arguments)
	}
	var parsedArgs map[string]any
	if err := json.Unmarshal([]byte(asst.ToolCalls[0].Arguments), &parsedArgs); err != nil || parsedArgs["city"] != "Bandung" {
		t.Errorf("arguments semantik salah: %s", asst.ToolCalls[0].Arguments)
	}
	tool := req.Messages[3]
	if tool.Role != RoleTool || tool.ToolCallID != "toolu_1" || tool.TextContent() != "24C cerah" {
		t.Errorf("tool = %+v", tool)
	}
	if len(req.Tools) != 1 || req.Tools[0].Parameters == nil {
		t.Errorf("tools = %+v", req.Tools)
	}
	if req.ToolChoice.Mode != ToolChoiceAuto {
		t.Errorf("tool_choice = %+v", req.ToolChoice)
	}
}

func TestParseAnthropicSystemBlocks(t *testing.T) {
	req, err := ParseAnthropicRequest([]byte(`{"model":"m","max_tokens":10,
		"system":[{"type":"text","text":"bagian1"},{"type":"text","text":"bagian2"}],
		"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(req.Messages[0].TextContent(), "bagian1") {
		t.Errorf("system = %q", req.Messages[0].TextContent())
	}
}

func TestRenderAnthropicRequestMergesRoles(t *testing.T) {
	req := &ChatRequest{
		Model:     "claude-x",
		MaxTokens: intp(512),
		Messages: []Message{
			{Role: RoleSystem, Content: []Part{{Type: PartText, Text: "sys"}}},
			{Role: RoleUser, Content: []Part{{Type: PartText, Text: "tanya"}}},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "t1", Name: "fn", Arguments: `{"a":1}`}}},
			{Role: RoleTool, ToolCallID: "t1", Content: []Part{{Type: PartText, Text: "hasil"}}},
			{Role: RoleAssistant, Content: []Part{{Type: PartText, Text: "jawab"}}},
		},
		Tools: []Tool{{Name: "fn", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	out, err := RenderAnthropicRequest(req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var w map[string]any
	if err := json.Unmarshal(out, &w); err != nil {
		t.Fatalf("json: %v (%s)", err, out)
	}
	if w["max_tokens"].(float64) != 512 {
		t.Errorf("max_tokens = %v", w["max_tokens"])
	}
	if w["system"] != "sys" {
		t.Errorf("system = %v", w["system"])
	}
	msgs := w["messages"].([]any)
	// user(tanya) | assistant(tool_use) | user(tool_result t1) | assistant(jawab)
	if len(msgs) != 4 {
		t.Fatalf("messages = %d: %s", len(msgs), out)
	}
	m1 := msgs[1].(map[string]any)
	if m1["role"] != "assistant" {
		t.Fatalf("m1 role = %v", m1["role"])
	}
	blocks := m1["content"].([]any)
	found := false
	for _, b := range blocks {
		bm := b.(map[string]any)
		if bm["type"] == "tool_use" && bm["id"] == "t1" {
			found = true
		}
	}
	if !found {
		t.Errorf("tool_use hilang: %s", out)
	}
	// tool_result masuk pesan user index 2
	m2u := msgs[2].(map[string]any)
	if m2u["role"] != "user" {
		t.Fatalf("m2 role = %v", m2u["role"])
	}
	foundTR := false
	for _, b := range m2u["content"].([]any) {
		bm := b.(map[string]any)
		if bm["type"] == "tool_result" && bm["tool_use_id"] == "t1" {
			foundTR = true
		}
	}
	if !foundTR {
		t.Errorf("tool_result hilang: %s", out)
	}
	m3 := msgs[3].(map[string]any)
	blocks3 := m3["content"].([]any)
	b3 := blocks3[0].(map[string]any)
	if b3["type"] != "text" || b3["text"] != "jawab" {
		t.Errorf("m3 = %v", b3)
	}
	tools := w["tools"].([]any)
	if tools[0].(map[string]any)["input_schema"] == nil {
		t.Error("input_schema hilang")
	}
}

func TestRenderAnthropicToolChoiceMapping(t *testing.T) {
	cases := []struct {
		tc   *ToolChoice
		want string
	}{
		{&ToolChoice{Mode: ToolChoiceAuto}, "auto"},
		{&ToolChoice{Mode: ToolChoiceRequired}, "any"},
		{&ToolChoice{Mode: ToolChoiceNamed, Name: "fn"}, "tool"},
	}
	for _, c := range cases {
		out, err := RenderAnthropicRequest(&ChatRequest{
			Model: "m", Messages: []Message{{Role: RoleUser, Content: []Part{{Type: PartText, Text: "x"}}}},
			ToolChoice: c.tc,
		})
		if err != nil {
			t.Fatal(err)
		}
		var w map[string]any
		json.Unmarshal(out, &w)
		var got string
		if v, ok := w["tool_choice"].(map[string]any); ok {
			got = v["type"].(string)
		}
		if got != c.want {
			t.Errorf("mode %v → %q, mau %q", c.tc.Mode, got, c.want)
		}
	}
	// none → tidak ada tool_choice dan tidak ada tools
	out, _ := RenderAnthropicRequest(&ChatRequest{
		Model: "m", Messages: []Message{{Role: RoleUser, Content: []Part{{Type: PartText, Text: "x"}}}},
		Tools: []Tool{{Name: "fn"}}, ToolChoice: &ToolChoice{Mode: ToolChoiceNone},
	})
	var w map[string]any
	json.Unmarshal(out, &w)
	if _, ok := w["tools"]; ok {
		t.Errorf("tool_choice=none harus membuang tools: %s", out)
	}
}

func TestRenderAnthropicImageDataURL(t *testing.T) {
	req := &ChatRequest{Model: "m", Messages: []Message{{Role: RoleUser, Content: []Part{
		{Type: PartImage, ImageURL: "data:image/png;base64,QUJD"},
	}}}}
	out, err := RenderAnthropicRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"media_type":"image/png"`) || !strings.Contains(string(out), `"data":"QUJD"`) {
		t.Errorf("image source salah: %s", out)
	}
}

func TestParseAnthropicResponse(t *testing.T) {
	body := []byte(`{
		"id": "msg_01", "type": "message", "role": "assistant", "model": "claude-x",
		"content": [
			{"type": "text", "text": "Jawaban"},
			{"type": "tool_use", "id": "toolu_9", "name": "fn", "input": {"x": 1}}
		],
		"stop_reason": "tool_use",
		"usage": {"input_tokens": 20, "output_tokens": 8}
	}`)
	resp, err := ParseAnthropicResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Jawaban" || resp.FinishReason != FinishToolCalls {
		t.Fatalf("resp = %+v", resp)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "fn" {
		t.Fatalf("toolcalls = %+v", resp.ToolCalls)
	}
	var tcArgs map[string]any
	if err := json.Unmarshal([]byte(resp.ToolCalls[0].Arguments), &tcArgs); err != nil || tcArgs["x"].(float64) != 1 {
		t.Errorf("arguments = %s", resp.ToolCalls[0].Arguments)
	}
	if resp.Usage.PromptTokens != 20 || resp.Usage.CompletionTokens != 8 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestParseAnthropicResponseError(t *testing.T) {
	_, err := ParseAnthropicResponse([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"lambat"}}`))
	if err == nil {
		t.Fatal("harus error")
	}
	ue := err.(*UpstreamError)
	if ue.Message != "lambat" || ue.Type != "rate_limit_error" {
		t.Fatalf("ue = %+v", ue)
	}
}

func TestRenderAnthropicResponseShape(t *testing.T) {
	out, err := RenderAnthropicResponse(&ChatResponse{
		ID: "msg_x", Model: "m", Content: "hai", FinishReason: FinishStop,
		Usage: Usage{PromptTokens: 3, CompletionTokens: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	var w map[string]any
	json.Unmarshal(out, &w)
	if w["type"] != "message" || w["stop_reason"] != "end_turn" {
		t.Fatalf("w = %v", w)
	}
	content := w["content"].([]any)
	if content[0].(map[string]any)["text"] != "hai" {
		t.Errorf("content = %v", content)
	}
}

func TestStopReasonMapping(t *testing.T) {
	pairs := map[string]string{
		"end_turn": FinishStop, "stop_sequence": FinishStop, "max_tokens": FinishLength,
		"tool_use": FinishToolCalls, "refusal": FinishContentFilter, "": FinishStop,
	}
	for a, b := range pairs {
		if StopReasonAnthropicToInternal(a) != b {
			t.Errorf("to internal %q → %q", a, StopReasonAnthropicToInternal(a))
		}
	}
	if StopReasonInternalToAnthropic(FinishToolCalls) != "tool_use" {
		t.Error("tool_calls → tool_use")
	}
	if StopReasonInternalToAnthropic(FinishLength) != "max_tokens" {
		t.Error("length → max_tokens")
	}
}

func TestRenderAnthropicErrorFormat(t *testing.T) {
	out := RenderAnthropicError("rate_limit_error", "lambat")
	var w struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(out, &w)
	if w.Type != "error" || w.Error.Type != "rate_limit_error" || w.Error.Message != "lambat" {
		t.Fatalf("w = %+v", w)
	}
	if MapStatusToAnthropicErrorType(429) != "rate_limit_error" {
		t.Error("map 429")
	}
}

func TestAnthropicSSEParserFullSequence(t *testing.T) {
	p := NewAnthropicSSEParser()
	frames := []string{
		`{"type":"message_start","message":{"id":"msg_1","model":"claude-x","usage":{"input_tokens":25,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Halo"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" dunia"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":12}}`,
		`{"type":"message_stop"}`,
	}
	var all []Event
	for _, f := range frames {
		evs, err := p.Parse([]byte(f))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		all = append(all, evs...)
	}
	// begin, delta, delta, end, usage
	if len(all) != 5 {
		t.Fatalf("events = %+v", all)
	}
	if all[0].Type != EventMessageBegin || all[0].ID != "msg_1" {
		t.Errorf("begin = %+v", all[0])
	}
	if all[1].Delta.Text != "Halo" || all[2].Delta.Text != " dunia" {
		t.Errorf("deltas = %+v", all[1:3])
	}
	if all[3].Type != EventMessageEnd || all[3].Delta.FinishReason != FinishStop {
		t.Errorf("end = %+v", all[3])
	}
	last := all[len(all)-1]
	if last.Type != EventUsage || last.Usage.PromptTokens != 25 || last.Usage.CompletionTokens != 12 {
		t.Errorf("usage = %+v", last)
	}
}

func TestAnthropicSSEParserToolUseSequence(t *testing.T) {
	p := NewAnthropicSSEParser()
	frames := []string{
		`{"type":"message_start","message":{"id":"msg_2","model":"m","usage":{"input_tokens":10,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Cek"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_7","name":"fn","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"a\":"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"1}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":30}}`,
		`{"type":"message_stop"}`,
	}
	var all []Event
	for _, f := range frames {
		evs, err := p.Parse([]byte(f))
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, evs...)
	}
	var args strings.Builder
	var id, name string
	for _, e := range all {
		if e.Type == EventDelta && len(e.Delta.ToolCalls) > 0 {
			tc := e.Delta.ToolCalls[0]
			if tc.ID != "" {
				id = tc.ID
			}
			if tc.Name != "" {
				name = tc.Name
			}
			args.WriteString(tc.Arguments)
		}
	}
	if id != "toolu_7" || name != "fn" {
		t.Errorf("id=%s name=%s", id, name)
	}
	if args.String() != `{"a":1}` {
		t.Errorf("args = %s", args.String())
	}
}

func TestAnthropicSSEWriterSequence(t *testing.T) {
	w := NewAnthropicSSEWriter()
	var all strings.Builder
	all.WriteString(w.Begin("msg_w", "claude-y", 9))
	for _, e := range []Event{
		{Type: EventDelta, Delta: &StreamDelta{Text: "Hai"}},
		{Type: EventDelta, Delta: &StreamDelta{Text: "!"}},
		{Type: EventMessageEnd, Delta: &StreamDelta{FinishReason: FinishStop}},
	} {
		for _, f := range w.WriteEvent(e) {
			all.WriteString(f)
		}
	}
	all.WriteString(w.Finish())
	s := all.String()
	for _, want := range []string{
		`event: message_start`, `"input_tokens":9`,
		`event: content_block_start`, `event: content_block_delta`, `"text":"Hai"`, `"type":"text_delta"`,
		`event: content_block_stop`,
		`event: message_delta`, `"stop_reason":"end_turn"`, `"output_tokens":0`,
		`event: message_stop`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("frame tidak memuat %q\n%s", want, s)
		}
	}
	// Finish kedua tidak boleh menghasilkan frame tambahan
	if s2 := w.Finish(); s2 != "" {
		t.Errorf("finish kedua harus kosong: %s", s2)
	}
}

func TestAnthropicSSEWriterToolUseFromOpenAIDeltas(t *testing.T) {
	// mensimulasikan: delta openai tool_calls → frame anthropic tool_use
	w := NewAnthropicSSEWriter()
	var s strings.Builder
	s.WriteString(w.Begin("", "m", 5))
	s.WriteString(w.WriteEvent(Event{Type: EventDelta, Delta: &StreamDelta{Text: "ok"}})[0])
	for _, f := range w.WriteEvent(Event{Type: EventDelta, Delta: &StreamDelta{
		ToolCalls: []ToolCallDelta{{Index: 0, ID: "call_1", Name: "fn"}},
	}}) {
		s.WriteString(f)
	}
	for _, f := range w.WriteEvent(Event{Type: EventDelta, Delta: &StreamDelta{
		ToolCalls: []ToolCallDelta{{Index: 0, Arguments: `{"x":`}},
	}}) {
		s.WriteString(f)
	}
	for _, f := range w.WriteEvent(Event{Type: EventDelta, Delta: &StreamDelta{
		ToolCalls: []ToolCallDelta{{Index: 0, Arguments: `1}`}},
	}}) {
		s.WriteString(f)
	}
	for _, f := range w.WriteEvent(Event{Type: EventMessageEnd, Delta: &StreamDelta{FinishReason: FinishToolCalls}}) {
		s.WriteString(f)
	}
	s.WriteString(w.Finish())
	out := s.String()
	for _, want := range []string{`"type":"tool_use"`, `"id":"call_1"`, `"name":"fn"`} {
		if !strings.Contains(out, want) {
			t.Errorf("tool_use start hilang %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, `"partial_json":"{\"x\":"`) {
		t.Errorf("input_json_delta hilang:\n%s", out)
	}
	if !strings.Contains(out, `"stop_reason":"tool_use"`) {
		t.Errorf("stop_reason salah:\n%s", out)
	}
	// blok teks harus ditutup sebelum tool_use dibuka
	if strings.Index(out, "content_block_stop") > strings.Index(out, `"type":"tool_use"`) {
		t.Errorf("urutan blok salah:\n%s", out)
	}
	if strings.Count(out, "event: message_stop") != 1 {
		t.Errorf("message_stop harus sekali:\n%s", out)
	}
	if again := w.Finish(); again != "" {
		t.Errorf("finish kedua harus kosong: %s", again)
	}
}

func TestAnthropicSSEWriterUsageAndErrorParse(t *testing.T) {
	p := NewAnthropicSSEParser()
	evs, err := p.Parse([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"penuh"}}`))
	if err != nil || len(evs) != 1 || evs[0].Type != EventError || evs[0].Error.Type != "overloaded_error" {
		t.Fatalf("evs=%+v err=%v", evs, err)
	}
	// ping diabaikan
	evs, _ = p.Parse([]byte(`{"type":"ping"}`))
	if len(evs) != 0 {
		t.Errorf("ping harus diabaikan: %+v", evs)
	}
}

func intp(i int) *int { return &i }
