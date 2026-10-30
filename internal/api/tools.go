package api

// Tool web_fetch untuk playground chat: model boleh meminta isi sebuah
// halaman web publik. Diambil SERVER-SIDE (bebas CORS), dengan guard SSRF
// (host internal/loopback/private ditolak) + batas ukuran + konversi
// HTML → teks sederhana.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jenderal/jenderalrouter/internal/provider"
	"github.com/jenderal/jenderalrouter/internal/websearch"
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

// errFetch error web_fetch yang aman ditampilkan ke pengguna/model.
type errFetch struct {
	status int
	msg    string
}

func (e *errFetch) Error() string { return e.msg }

// fetchPage mengambil halaman publik lalu mengubahnya menjadi teks
// (dipakai endpoint tool dan agen chat server-side).
func (a *App) fetchPage(ctx context.Context, target string) (title, text string, err error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", &errFetch{400, "url wajib"}
	}
	// guard SSRF: tolak host internal/loopback/private — tool ini hanya
	// untuk membaca situs publik, bukan service internal server.
	if err := provider.CheckBaseURL(target, false, false); err != nil {
		return "", "", &errFetch{400, err.Error()}
	}
	// klien publik: IP diperiksa saat dial (redirect & DNS rebinding ikut terjaga)
	client := provider.NewPublicHTTPClient(30 * time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", "", &errFetch{400, "URL tidak sah: " + err.Error()}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; JenderalRouterBot/1.0; +webfetch tool)")
	resp, err := client.Do(req)
	if err != nil {
		msg := "gagal mengakses situs"
		if errors.Is(err, provider.ErrSSRFBlocked) {
			msg = "alamat internal/privat tidak diizinkan"
		}
		return "", "", &errFetch{http.StatusBadGateway, msg}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", &errFetch{http.StatusBadGateway, fmt.Sprintf("situs merespons HTTP %d", resp.StatusCode)}
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	text = string(raw)
	isHTML := strings.Contains(ct, "html") || strings.HasPrefix(strings.TrimSpace(text), "<")
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
	return title, text, nil
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
	title, text, err := a.fetchPage(r.Context(), target)
	if err != nil {
		status := http.StatusBadGateway
		var fe *errFetch
		if errors.As(err, &fe) {
			status = fe.status
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "tool.webfetch", target, nil, map[string]int{"chars": len([]rune(text))})
	writeJSON(w, http.StatusOK, map[string]any{
		"url": target, "title": title, "chars": len([]rune(text)), "text": text,
	})
}

// searchError pesan error pencarian yang ramah untuk pengguna/model.
func (a *App) searchError(err error) string {
	msg := "pencarian gagal"
	switch {
	case errors.Is(err, provider.ErrSSRFBlocked):
		msg = "alamat internal/privat tidak diizinkan"
	case errors.Is(err, websearch.ErrRateLimited) && a.search.Name() == "brave-html":
		msg += ": " + err.Error() + ". Admin dapat mengatur JR_SEARXNG_URL atau JR_BRAVE_API_KEY agar pencarian lebih andal"
	case !errors.Is(err, context.Canceled):
		msg += ": " + err.Error()
	}
	return msg
}

// handleToolWebSearch POST /api/me/tools/websearch — {query, count}.
func (a *App) handleToolWebSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
		Count int    `json:"count"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	q := strings.TrimSpace(req.Query)
	if q == "" {
		writeJSON(w, 400, map[string]string{"error": "query wajib"})
		return
	}
	if r := []rune(q); len(r) > 400 {
		q = string(r[:400])
	}
	if req.Count <= 0 || req.Count > 10 {
		req.Count = 6
	}
	results, err := a.search.Search(r.Context(), q, req.Count)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": a.searchError(err)})
		return
	}
	a.audit(r, "tool.websearch", q, nil, map[string]any{"backend": a.search.Name(), "results": len(results)})
	writeJSON(w, http.StatusOK, map[string]any{"query": q, "backend": a.search.Name(), "results": results})
}
