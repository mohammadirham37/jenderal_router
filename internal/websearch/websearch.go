// Package websearch menyediakan pencarian web untuk tool web_search chat.
// Beberapa backend didukung; yang dipakai ditentukan konfigurasi:
// SearXNG (self-host), Brave Search API, Tavily, atau halaman HTML Brave
// Search sebagai fallback tanpa API key.
package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Result satu hasil pencarian.
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// Searcher backend pencarian.
type Searcher interface {
	Name() string
	Search(ctx context.Context, query string, count int) ([]Result, error)
}

// Config pilihan backend; field kosong = backend tidak aktif.
type Config struct {
	SearXNGURL   string
	BraveAPIKey  string
	TavilyAPIKey string
}

// Endpoint publik — variabel agar bisa diarahkan ke server uji.
var (
	braveURL     = "https://api.search.brave.com/res/v1/web/search"
	tavilyURL    = "https://api.tavily.com/search"
	braveHTMLURL = "https://search.brave.com/search"
)

// halaman HTML Brave menolak UA bot, jadi pakai UA browser umum
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

// New memilih backend sesuai konfigurasi. client dipakai untuk API publik
// (Brave/Tavily/Brave HTML); SearXNG dikonfigurasi admin dan boleh berada
// di jaringan privat, jadi memakai klien biasa.
func New(cfg Config, client *http.Client) Searcher {
	switch {
	case cfg.SearXNGURL != "":
		return &searxng{base: strings.TrimRight(cfg.SearXNGURL, "/"), client: &http.Client{Timeout: 20 * time.Second}}
	case cfg.BraveAPIKey != "":
		return &brave{key: cfg.BraveAPIKey, client: client}
	case cfg.TavilyAPIKey != "":
		return &tavily{key: cfg.TavilyAPIKey, client: client}
	default:
		return &braveHTML{client: client}
	}
}

// ErrRateLimited mesin pencari menolak karena terlalu banyak permintaan.
var ErrRateLimited = errors.New("mesin pencari membatasi permintaan (HTTP 429), coba lagi sebentar")

func do(client *http.Client, req *http.Request, v any) error {
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode == http.StatusTooManyRequests {
		return ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mesin pencari merespons HTTP %d", resp.StatusCode)
	}
	if p, ok := v.(*[]byte); ok {
		*p = body
		return nil
	}
	return json.Unmarshal(body, v)
}

func limit(rs []Result, n int) []Result {
	out := rs[:0]
	for _, r := range rs {
		r.Title = strings.TrimSpace(stripTags(r.Title))
		r.Snippet = strings.TrimSpace(stripTags(r.Snippet))
		if r.URL == "" || r.Title == "" {
			continue
		}
		out = append(out, r)
		if len(out) == n {
			break
		}
	}
	return out
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

func stripTags(s string) string { return html.UnescapeString(tagRe.ReplaceAllString(s, "")) }

// ---- SearXNG ----

type searxng struct {
	base   string
	client *http.Client
}

func (s *searxng) Name() string { return "searxng" }

func (s *searxng) Search(ctx context.Context, q string, n int) ([]Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		s.base+"/search?format=json&q="+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := do(s.client, req, &out); err != nil {
		return nil, err
	}
	rs := make([]Result, 0, len(out.Results))
	for _, r := range out.Results {
		rs = append(rs, Result{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return limit(rs, n), nil
}

// ---- Brave Search API ----

type brave struct {
	key    string
	client *http.Client
}

func (b *brave) Name() string { return "brave" }

func (b *brave) Search(ctx context.Context, q string, n int) ([]Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s?q=%s&count=%d", braveURL, url.QueryEscape(q), n), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.key)
	var out struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := do(b.client, req, &out); err != nil {
		return nil, err
	}
	rs := make([]Result, 0, len(out.Web.Results))
	for _, r := range out.Web.Results {
		rs = append(rs, Result{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return limit(rs, n), nil
}

// ---- Tavily ----

type tavily struct {
	key    string
	client *http.Client
}

func (t *tavily) Name() string { return "tavily" }

func (t *tavily) Search(ctx context.Context, q string, n int) ([]Result, error) {
	body, _ := json.Marshal(map[string]any{"query": q, "max_results": n})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilyURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.key)
	var out struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := do(t.client, req, &out); err != nil {
		return nil, err
	}
	rs := make([]Result, 0, len(out.Results))
	for _, r := range out.Results {
		rs = append(rs, Result{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return limit(rs, n), nil
}

// ---- Brave Search HTML (fallback tanpa API key) ----
// DuckDuckGo diblokir di Indonesia dan Bing memberi hasil acak ke klien
// non-browser, jadi fallback memakai halaman hasil search.brave.com.
// Rapuh terhadap perubahan markup — untuk produksi pakai SearXNG/API key.

type braveHTML struct{ client *http.Client }

func (b *braveHTML) Name() string { return "brave-html" }

var (
	bhLinkRe    = regexp.MustCompile(`<a href="(https?://[^"]+)"`)
	bhTitleRe   = regexp.MustCompile(`class="title[^"]*"[^>]*title="([^"]*)"`)
	bhSnippetRe = regexp.MustCompile(`(?s)class="content[^"]*">(.*?)</div>`)
)

func (b *braveHTML) Search(ctx context.Context, q string, n int) ([]Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, braveHTMLURL+"?q="+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html")
	var body []byte
	if err := do(b.client, req, &body); err != nil {
		return nil, err
	}
	return parseBraveHTML(string(body), n), nil
}

// parseBraveHTML mengurai blok hasil web (data-type="web") halaman Brave.
func parseBraveHTML(page string, n int) []Result {
	parts := strings.Split(page, `data-type="web"`)
	rs := make([]Result, 0, len(parts))
	for _, p := range parts[1:] {
		link := bhLinkRe.FindStringSubmatch(p)
		title := bhTitleRe.FindStringSubmatch(p)
		if link == nil || title == nil {
			continue
		}
		r := Result{URL: html.UnescapeString(link[1]), Title: title[1]}
		if sn := bhSnippetRe.FindStringSubmatch(p); sn != nil {
			r.Snippet = sn[1]
		}
		rs = append(rs, r)
	}
	return limit(rs, n)
}
