package api

// Tool web_fetch untuk playground chat: model boleh meminta isi sebuah
// halaman web publik. Diambil SERVER-SIDE (bebas CORS), dengan guard SSRF
// (host internal/loopback/private ditolak) + batas ukuran + konversi
// HTML → teks sederhana.

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jenderal/jenderalrouter/internal/provider"
)

var stripBlockRe = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<script[^>]*>[\s\S]*?</script>`),
	regexp.MustCompile(`(?is)<style[^>]*>[\s\S]*?</style>`),
	regexp.MustCompile(`(?is)<noscript[^>]*>[\s\S]*?</noscript>`),
	regexp.MustCompile(`(?is)<svg[^>]*>[\s\S]*?</svg>`),
	regexp.MustCompile(`(?is)<iframe[^>]*>[\s\S]*?</iframe>`),
}
var stripNavRe = regexp.MustCompile(`(?is)<nav[^>]*>[\s\S]*?</nav>`)
var stripFooterRe = regexp.MustCompile(`(?is)<footer[^>]*>[\s\S]*?</footer>`)
var stripFormRe = regexp.MustCompile(`(?is)<form[^>]*>[\s\S]*?</form>`)
var tagRe = regexp.MustCompile(`(?s)<[^>]+>`)
var spaceRe = regexp.MustCompile(`\n{3,}`)

// htmlToText konversi HTML sederhana menjadi teks terbaca.
func htmlToText(html string) string {
	s := html
	for _, re := range append(stripBlockRe, stripNavRe, stripFooterRe, stripFormRe) {
		s = re.ReplaceAllString(s, " ")
	}
	s = tagRe.ReplaceAllString(s, " ")
	for _, pair := range [][2]string{
		{"&nbsp;", " "}, {"&amp;", "&"}, {"&lt;", "<"}, {"&gt;", ">"},
		{"&quot;", "\""}, {"&#39;", "'"}, {"&apos;", "'"}, {"&mdash;", "—"},
	} {
		s = strings.ReplaceAll(s, pair[0], pair[1])
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	s = strings.Join(lines, "\n")
	s = spaceRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func extractTitle(html string) string {
	re := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(htmlToText(m[1]))
}

// handleToolWebFetch POST /api/me/tools/webfetch — {url: "..."}.
func (a *App) handleToolWebFetch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	target := strings.TrimSpace(req.URL)
	if target == "" {
		writeJSON(w, 400, map[string]string{"error": "url wajib"})
		return
	}
	// guard SSRF: tolak host internal/loopback/private — tool ini hanya
	// untuk membaca situs publik, bukan service internal server.
	if err := provider.CheckBaseURL(target, false, false); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req2, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "URL tidak sah: " + err.Error()})
		return
	}
	req2.Header.Set("User-Agent", "Mozilla/5.0 (compatible; JenderalRouterBot/1.0; +webfetch tool)")
	resp, err := client.Do(req2)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "gagal mengakses situs: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("situs merespons HTTP %d", resp.StatusCode)})
		return
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	text := string(raw)
	isHTML := strings.Contains(ct, "html") || strings.HasPrefix(strings.TrimSpace(text), "<")
	title := ""
	if isHTML {
		title = extractTitle(text)
		text = htmlToText(text)
	}
	if runes := []rune(text); len(runes) > 6000 {
		text = string(runes[:6000]) + "… [dipotong]"
	}
	if strings.TrimSpace(text) == "" {
		text = "(halaman tidak memiliki teks yang bisa dibaca)"
	}
	a.audit(r, "tool.webfetch", target, nil, map[string]int{"chars": len([]rune(text))})
	writeJSON(w, http.StatusOK, map[string]any{
		"url": target, "title": title, "chars": len([]rune(text)), "text": text,
	})
}
