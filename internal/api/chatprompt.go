package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/translate"
)

// System prompt dasar chat playground. Disuntik server-side agar setiap
// model (lokal maupun cloud) tahu konteks: tanggal, pengguna, gaya jawaban,
// dan cara memakai tool — tanpa ini model kecil sering mengarang tanggal
// atau mengabaikan tool.

var hariID = [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
var bulanID = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli",
	"Agustus", "September", "Oktober", "November", "Desember"}

// tanggalID format tanggal-jam Indonesia, mis. "Kamis, 30 Oktober 2026 14:05 WIB".
func tanggalID(t time.Time) string {
	return fmt.Sprintf("%s, %d %s %d %02d:%02d %s", hariID[t.Weekday()], t.Day(),
		bulanID[t.Month()-1], t.Year(), t.Hour(), t.Minute(), t.Format("MST"))
}

const chatGuidelines = `Pedoman menjawab:
- Jawab dalam bahasa yang dipakai pengguna. Langsung ke inti; tanpa basa-basi pembuka atau penutup.
- Sesuaikan panjang dengan pertanyaan: pertanyaan singkat dijawab singkat, tugas kompleks dijawab lengkap dan terstruktur.
- Gunakan Markdown bila membantu: heading untuk jawaban panjang, daftar untuk langkah, tabel untuk perbandingan, blok kode berlabel bahasa untuk kode. Jangan memformat berlebihan untuk jawaban pendek.
- Jujur: bila tidak tahu atau tidak yakin, katakan. Jangan mengarang fakta, angka, kutipan, nama, atau URL.
- Pengetahuan Anda punya batas waktu pelatihan. Untuk peristiwa atau data terbaru, akui keterbatasan itu atau gunakan tool bila tersedia.
- Untuk permintaan yang ambigu, pilih tafsiran paling masuk akal lalu sebutkan asumsinya; bertanya balik hanya bila benar-benar perlu.
- Untuk kode: tulis kode lengkap yang bisa langsung dijalankan, lalu jelaskan singkat bagian pentingnya.`

const chatToolGuidelines = `Pemakaian tool:
- Panggil tool bila memang dibutuhkan untuk menjawab dengan benar; jangan untuk hal yang sudah Anda ketahui pasti.
- web_fetch membaca halaman web publik. Gunakan saat pengguna memberi URL, atau saat butuh informasi terkini dan Anda tahu URL sumber yang tepat. Boleh dipanggil beberapa kali.
- Hasil tool dari giliran sebelumnya tetap ada di riwayat; pakai ulang, jangan mengambil halaman yang sama lagi tanpa alasan.
- Saat memakai hasil web, sebutkan sumbernya (judul atau URL). Bila tool gagal, beri tahu pengguna dan jawab sebaik mungkin dari pengetahuan Anda.`

// chatSystemPrompt merakit system prompt untuk satu request chat.
func (a *App) chatSystemPrompt(u *store.User, tools []translate.Tool, custom string) string {
	loc, err := time.LoadLocation(a.cfg.TZ)
	if err != nil {
		loc = time.Local
	}
	var b strings.Builder
	b.WriteString("Anda adalah asisten AI yang cerdas dan membantu di aplikasi JenderalRouter.\n")
	b.WriteString("Waktu sekarang: " + tanggalID(time.Now().In(loc)) + ".\n")
	if u != nil && u.Email != "" {
		b.WriteString("Pengguna: " + u.Email + ".\n")
	}
	b.WriteString("\n" + chatGuidelines + "\n")
	if len(tools) > 0 {
		names := make([]string, 0, len(tools))
		for _, t := range tools {
			names = append(names, t.Name)
		}
		b.WriteString("\nTool tersedia: " + strings.Join(names, ", ") + ".\n" + chatToolGuidelines + "\n")
	}
	if c := strings.TrimSpace(custom); c != "" {
		b.WriteString("\nInstruksi khusus dari pengguna untuk percakapan ini (utamakan):\n" + c + "\n")
	}
	return b.String()
}

// withChatSystemPrompt menaruh prompt dasar di depan dan melebur system
// message bawaan klien ke dalamnya — sebagian chat template (llama.cpp)
// menolak system message yang bukan pesan pertama.
func withChatSystemPrompt(msgs []translate.Message, base string) []translate.Message {
	texts := []string{base}
	i := 0
	for ; i < len(msgs) && msgs[i].Role == translate.RoleSystem; i++ {
		if t := strings.TrimSpace(msgs[i].TextContent()); t != "" {
			texts = append(texts, t)
		}
	}
	sys := translate.Message{
		Role:    translate.RoleSystem,
		Content: []translate.Part{{Type: translate.PartText, Text: strings.Join(texts, "\n\n")}},
	}
	return append([]translate.Message{sys}, msgs[i:]...)
}
