package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/translate"
)

// Riwayat percakapan untuk model, dibangun dari DB (pesan + meta). Format
// meta sama dengan yang ditulis UI chat sebelumnya, jadi percakapan lama
// tetap terbaca.

const (
	keepToolTurns  = 3 // giliran bertool terakhir yang membawa hasil tool
	keepImageTurns = 2 // giliran bergambar terakhir yang mengirim gambar utuh
)

// toolUse satu pemanggilan tool dalam giliran assistant.
type toolUse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Args   string `json:"args"`
	Round  int    `json:"round"`
	Status string `json:"status"` // run|ok|err
	Result string `json:"result,omitempty"`
	URL    string `json:"url,omitempty"`
	Query  string `json:"query,omitempty"`
	Hits   *int   `json:"hits,omitempty"`
	Doc    string `json:"doc,omitempty"`
}

// attachment lampiran pesan user (teks hasil ekstraksi / gambar kecil).
type attachment struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Kind      string `json:"kind"` // image|pdf|docx|xlsx|pptx|text
	DataURL   string `json:"dataUrl,omitempty"`
	Text      string `json:"text,omitempty"`
	Pages     int    `json:"pages,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// docInfo dokumen yang dirakit UI dari jawaban (start = offset UTF-16).
type docInfo struct {
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
	Start int    `json:"start"`
}

// messageMeta isi kolom messages.meta.
type messageMeta struct {
	Reasoning      string       `json:"reasoning,omitempty"`
	ThinkMs        int64        `json:"think_ms,omitempty"`
	Tools          []toolUse    `json:"tools,omitempty"`
	Stopped        bool         `json:"stopped,omitempty"`
	Attachments    []attachment `json:"attachments,omitempty"`
	ContextTrimmed int          `json:"context_trimmed,omitempty"`
	ContextWindow  int          `json:"context_window,omitempty"`
	Doc            *docInfo     `json:"doc,omitempty"`
	Error          string       `json:"error,omitempty"`
}

func parseMeta(raw json.RawMessage) messageMeta {
	var m messageMeta
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}

var kindLabel = map[string]string{
	"image": "gambar", "pdf": "PDF", "docx": "Word", "xlsx": "Excel", "pptx": "PowerPoint", "text": "teks",
}

func docBlock(a attachment) string {
	attrs := []string{
		fmt.Sprintf(`nama="%s"`, strings.ReplaceAll(a.Name, `"`, "'")),
		fmt.Sprintf(`jenis="%s"`, kindLabel[a.Kind]),
	}
	if a.Pages > 0 {
		unit := "halaman"
		switch a.Kind {
		case "pptx":
			unit = "slide"
		case "xlsx":
			unit = "sheet"
		}
		attrs = append(attrs, fmt.Sprintf(`%s="%d"`, unit, a.Pages))
	}
	note := ""
	if a.Truncated {
		note = "\n[... isi dipotong karena terlalu panjang]"
	}
	return "<lampiran " + strings.Join(attrs, " ") + ">\n" + a.Text + note + "\n</lampiran>"
}

// userMessage merakit pesan user: dokumen di depan pertanyaan; gambar
// sebagai part vision, atau catatan teks bila withImages=false.
func userMessage(text string, atts []attachment, withImages bool) translate.Message {
	var docs, imgs []attachment
	for _, a := range atts {
		switch {
		case a.Kind == "image" && a.DataURL != "":
			imgs = append(imgs, a)
		case a.Kind != "image" && a.Text != "":
			docs = append(docs, a)
		}
	}
	if len(docs) == 0 && len(imgs) == 0 {
		return translate.Message{Role: translate.RoleUser, Content: []translate.Part{{Type: translate.PartText, Text: text}}}
	}
	var blocks []string
	for _, d := range docs {
		blocks = append(blocks, docBlock(d))
	}
	if len(imgs) > 0 && !withImages {
		for _, im := range imgs {
			blocks = append(blocks, "[gambar terlampir sebelumnya: "+im.Name+"]")
		}
	}
	q := text
	if q == "" {
		q = "Pelajari lampiran ini."
		if len(imgs) > 0 {
			q = "Jelaskan gambar ini."
		}
	}
	blocks = append(blocks, q)
	parts := []translate.Part{{Type: translate.PartText, Text: strings.Join(blocks, "\n\n")}}
	if withImages {
		for _, im := range imgs {
			parts = append(parts, translate.Part{Type: translate.PartImage, ImageURL: im.DataURL})
		}
	}
	return translate.Message{Role: translate.RoleUser, Content: parts}
}

// toolMessages pemanggilan tool + hasilnya, per putaran.
func toolMessages(tools []toolUse) []translate.Message {
	var out []translate.Message
	var rounds []int
	seen := map[int]bool{}
	for _, t := range tools {
		if !seen[t.Round] {
			seen[t.Round] = true
			rounds = append(rounds, t.Round)
		}
	}
	for _, r := range rounds {
		var calls []translate.ToolCall
		var results []translate.Message
		for _, t := range tools {
			if t.Round != r || t.Status == "run" {
				continue
			}
			args := t.Args
			if args == "" {
				args = "{}"
			}
			calls = append(calls, translate.ToolCall{ID: t.ID, Name: t.Name, Arguments: args})
			results = append(results, translate.Message{Role: translate.RoleTool, ToolCallID: t.ID,
				Content: []translate.Part{{Type: translate.PartText, Text: t.Result}}})
		}
		if len(calls) == 0 {
			continue
		}
		out = append(out, translate.Message{Role: translate.RoleAssistant, ToolCalls: calls})
		out = append(out, results...)
	}
	return out
}

// historyFromMessages riwayat model dari pesan tersimpan.
func historyFromMessages(msgs []*store.Message) []translate.Message {
	metas := make([]messageMeta, len(msgs))
	for i, m := range msgs {
		metas[i] = parseMeta(m.Meta)
	}
	keepTools := map[int]bool{}
	keepImages := map[int]bool{}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" && len(metas[i].Tools) > 0 && len(keepTools) < keepToolTurns {
			keepTools[i] = true
		}
		if msgs[i].Role == "user" && len(keepImages) < keepImageTurns {
			for _, a := range metas[i].Attachments {
				if a.Kind == "image" {
					keepImages[i] = true
					break
				}
			}
		}
	}
	var out []translate.Message
	for i, m := range msgs {
		switch m.Role {
		case "user":
			out = append(out, userMessage(m.Content, metas[i].Attachments, keepImages[i]))
		case "assistant":
			if keepTools[i] {
				out = append(out, toolMessages(metas[i].Tools)...)
			}
			if strings.TrimSpace(m.Content) != "" {
				out = append(out, translate.Message{Role: translate.RoleAssistant,
					Content: []translate.Part{{Type: translate.PartText, Text: m.Content}}})
			}
		}
	}
	return out
}
