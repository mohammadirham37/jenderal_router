package api

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jenderal/jenderalrouter/internal/router"
	"github.com/jenderal/jenderalrouter/internal/translate"
)

// Pengelolaan konteks chat playground: riwayat dipangkas di server agar
// muat di context window model yang benar-benar melayani (combo: jendela
// terkecil di antara langkahnya). Urutan pemangkasan dari yang paling
// murah kehilangannya:
//  1. isi lampiran & gambar pada giliran lama,
//  2. giliran paling awal (utuh per giliran agar pasangan tool tetap sah),
//  3. potong teks terpanjang yang tersisa.

const (
	defaultContextWindow = 32768 // bila admin belum mengisi context window model
	imageTokenCost       = 800   // perkiraan kasar token per gambar
	keepFullUserTurns    = 2     // lampiran utuh hanya untuk N pesan user terakhir
	oldAttachmentChars   = 1500  // sisa isi lampiran lama setelah dipangkas
)

// chatContextBudget batas token input untuk langkah-langkah rute: context
// window terkecil dikurangi cadangan jawaban.
func (a *App) chatContextBudget(steps []router.Step) (budget, window int) {
	for _, s := range steps {
		m, err := a.st.GetModel(s.ModelID)
		if err != nil || m.ContextWindow <= 0 {
			continue
		}
		if window == 0 || m.ContextWindow < window {
			window = m.ContextWindow
		}
	}
	if window == 0 {
		window = defaultContextWindow
	}
	reserve := min(window/4, 8192)
	return window - reserve, window
}

func estimateMessage(m translate.Message) int {
	n := 4
	for _, p := range m.Content {
		if p.Type == translate.PartImage {
			n += imageTokenCost
		} else {
			n += translate.EstimateTokens(p.Text)
		}
	}
	for _, tc := range m.ToolCalls {
		n += translate.EstimateTokens(tc.Name+tc.Arguments) + 3
	}
	return n
}

func estimateAll(msgs []translate.Message) int {
	t := 0
	for _, m := range msgs {
		t += estimateMessage(m)
	}
	return t
}

var lampiranRe = regexp.MustCompile(`(?s)(<lampiran [^>]*>\n)(.*?)(\n</lampiran>)`)

// shrinkAttachments memangkas isi blok <lampiran> dan mengganti gambar
// dengan catatan teks pada pesan user lama.
func shrinkAttachments(m *translate.Message) bool {
	changed := false
	parts := make([]translate.Part, 0, len(m.Content))
	images := 0
	for _, p := range m.Content {
		if p.Type == translate.PartImage {
			images++
			changed = true
			continue
		}
		p.Text = lampiranRe.ReplaceAllStringFunc(p.Text, func(block string) string {
			sm := lampiranRe.FindStringSubmatch(block)
			body := []rune(sm[2])
			if len(body) <= oldAttachmentChars {
				return block
			}
			changed = true
			return sm[1] + string(body[:oldAttachmentChars]) + "\n[... isi lampiran dipangkas karena batas konteks model]" + sm[3]
		})
		parts = append(parts, p)
	}
	if images > 0 {
		parts = append(parts, translate.Part{Type: translate.PartText,
			Text: fmt.Sprintf("[%d gambar dari giliran sebelumnya tidak disertakan karena batas konteks]", images)})
	}
	m.Content = parts
	return changed
}

// trimForContext memangkas riwayat agar estimasi token <= budget. Pesan
// system di depan dan pesan user terakhir selalu dipertahankan. Mengembalikan
// riwayat baru dan jumlah pesan lama yang dibuang.
func trimForContext(msgs []translate.Message, budget int) ([]translate.Message, int) {
	if estimateAll(msgs) <= budget {
		return msgs, 0
	}
	out := make([]translate.Message, len(msgs))
	copy(out, msgs)
	for i := range out { // salin slice konten agar input tidak termutasi
		out[i].Content = append([]translate.Part(nil), out[i].Content...)
	}

	sys := 0
	for sys < len(out) && out[sys].Role == translate.RoleSystem {
		sys++
	}
	userIdx := []int{}
	for i := sys; i < len(out); i++ {
		if out[i].Role == translate.RoleUser {
			userIdx = append(userIdx, i)
		}
	}

	// 1. lampiran & gambar lama
	for k := 0; k < len(userIdx)-keepFullUserTurns; k++ {
		shrinkAttachments(&out[userIdx[k]])
	}
	if estimateAll(out) <= budget {
		return out, 0
	}

	// 2. buang giliran terawal (user + jawaban + tool-nya) selama masih ada >1 giliran
	head, body := out[:sys], out[sys:]
	dropped := 0
	var topics []string
	for estimateAll(head)+estimateAll(body) > budget {
		next := -1
		for i := 1; i < len(body); i++ {
			if body[i].Role == translate.RoleUser {
				next = i
				break
			}
		}
		if next < 0 {
			break // tinggal giliran terakhir
		}
		if body[0].Role == translate.RoleUser && len(topics) < 5 {
			if t := strings.TrimSpace(lampiranRe.ReplaceAllString(body[0].TextContent(), "")); t != "" {
				r := []rune(strings.Join(strings.Fields(t), " "))
				if len(r) > 80 {
					r = append(r[:80], '…')
				}
				topics = append(topics, string(r))
			}
		}
		dropped += next
		body = body[next:]
	}
	if dropped > 0 {
		note := fmt.Sprintf("Catatan: %d pesan awal percakapan ini tidak disertakan karena batas konteks model.", dropped)
		if len(topics) > 0 {
			note += " Topik yang dibahas sebelumnya: " + strings.Join(topics, "; ") + "."
		}
		if len(head) > 0 {
			h := head[0]
			h.Content = append(append([]translate.Part(nil), h.Content...), translate.Part{Type: translate.PartText, Text: "\n\n" + note})
			head = append([]translate.Message{h}, head[1:]...)
		} else {
			head = []translate.Message{{Role: translate.RoleSystem, Content: []translate.Part{{Type: translate.PartText, Text: note}}}}
		}
	}
	out = append(append([]translate.Message(nil), head...), body...)

	// 3. masih kelebihan (giliran terakhir sendiri terlalu besar): potong
	// teks terpanjang di luar pesan system
	for range 8 {
		over := estimateAll(out) - budget
		if over <= 0 {
			break
		}
		mi, pi, longest := -1, -1, 0
		for i := range out {
			if out[i].Role == translate.RoleSystem {
				continue
			}
			for j, p := range out[i].Content {
				if p.Type == translate.PartText && len(p.Text) > longest {
					mi, pi, longest = i, j, len(p.Text)
				}
			}
		}
		if mi < 0 {
			break
		}
		r := []rune(out[mi].Content[pi].Text)
		keep := len(r) - over*4 - 200 // ~4 karakter per token + margin
		if keep < 200 {
			keep = min(200, len(r))
		}
		out[mi].Content[pi].Text = string(r[:keep]) + "\n[... dipotong karena batas konteks model]"
		if keep == len(r) {
			break
		}
	}
	return out, dropped
}
