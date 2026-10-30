package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseBraveHTML(t *testing.T) {
	page := `<div>header</div>
<div class="snippet svelte-x" data-pos="0" data-type="web"><a href="https://go.dev/dl/" target="_self" class="l1"><cite>go.dev</cite>
<div class="title search-snippet-title line-clamp-1" title="All releases - The Go &amp; Language">All releases</div></a>
<div class="generic-snippet"><div class="content desktop-default-regular t-primary"><span class="t-secondary">August 21, 2026 -</span> See the <strong>release</strong> history&nbsp;...</div></div></div>
<div class="snippet" data-pos="1" data-type="web"><a href="https://example.com/no-title">tanpa judul</a></div>
<div class="snippet" data-pos="2" data-type="web"><a href="https://go.dev/doc/devel/release" class="l1">
<div class="title search-snippet-title" title="Release History">Release History</div></a></div>`
	got := parseBraveHTML(page, 5)
	if len(got) != 2 {
		t.Fatalf("want 2 results, got %d: %+v", len(got), got)
	}
	if got[0].URL != "https://go.dev/dl/" || got[0].Title != "All releases - The Go & Language" {
		t.Errorf("first result = %+v", got[0])
	}
	if want := "August 21, 2026 - See the release history ..."; got[0].Snippet != want {
		t.Errorf("snippet = %q, want %q", got[0].Snippet, want)
	}
	if got[1].URL != "https://go.dev/doc/devel/release" || got[1].Snippet != "" {
		t.Errorf("second result = %+v", got[1])
	}
	if one := parseBraveHTML(page, 1); len(one) != 1 {
		t.Errorf("count limit ignored: %d", len(one))
	}
}

func TestSearXNG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" || r.URL.Query().Get("format") != "json" || r.URL.Query().Get("q") != "go 1.30" {
			t.Errorf("unexpected request %s", r.URL)
		}
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]string{
			{"title": "Go 1.30", "url": "https://go.dev/doc/go1.30", "content": "Rilis <b>baru</b>"},
		}})
	}))
	defer srv.Close()
	s := New(Config{SearXNGURL: srv.URL + "/"}, srv.Client())
	rs, err := s.Search(context.Background(), "go 1.30", 5)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name() != "searxng" || len(rs) != 1 || rs[0].Snippet != "Rilis baru" {
		t.Errorf("%s results = %+v", s.Name(), rs)
	}
}

func TestBraveAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "k1" || r.URL.Query().Get("count") != "3" {
			t.Errorf("unexpected request %s %v", r.URL, r.Header)
		}
		json.NewEncoder(w).Encode(map[string]any{"web": map[string]any{"results": []map[string]string{
			{"title": "A", "url": "https://a.test", "description": "da"},
			{"title": "", "url": "https://kosong.test"},
		}}})
	}))
	defer srv.Close()
	old := braveURL
	braveURL = srv.URL
	defer func() { braveURL = old }()
	rs, err := New(Config{BraveAPIKey: "k1", TavilyAPIKey: "x"}, srv.Client()).Search(context.Background(), "a", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].URL != "https://a.test" {
		t.Errorf("results = %+v", rs)
	}
}

func TestTavily(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if r.Header.Get("Authorization") != "Bearer tk" || body["query"] != "q" {
			t.Errorf("unexpected request %v %v", r.Header, body)
		}
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]string{{"title": "T", "url": "https://t.test", "content": "c"}}})
	}))
	defer srv.Close()
	old := tavilyURL
	tavilyURL = srv.URL
	defer func() { tavilyURL = old }()
	rs, err := New(Config{TavilyAPIKey: "tk"}, srv.Client()).Search(context.Background(), "q", 5)
	if err != nil || len(rs) != 1 || rs[0].Snippet != "c" {
		t.Fatalf("results = %+v, err %v", rs, err)
	}
}

func TestHTTPErrorSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	old := braveHTMLURL
	braveHTMLURL = srv.URL // cadangan ikut gagal → error diteruskan
	defer func() { braveHTMLURL = old }()
	if _, err := New(Config{SearXNGURL: srv.URL}, nil).Search(context.Background(), "q", 5); err == nil {
		t.Fatal("want error on HTTP 503")
	}
}

type countSearcher struct{ calls int }

func (c *countSearcher) Name() string { return "count" }
func (c *countSearcher) Search(_ context.Context, q string, n int) ([]Result, error) {
	c.calls++
	if q == "kosong" {
		return nil, nil
	}
	return []Result{{Title: "a", URL: "https://a"}, {Title: "b", URL: "https://b"}}[:n], nil
}

func TestCached(t *testing.T) {
	inner := &countSearcher{}
	c := Cached(inner, time.Minute, 2)
	ctx := context.Background()
	c.Search(ctx, "Harga  Beras", 2)
	if rs, _ := c.Search(ctx, "harga beras", 1); len(rs) != 1 || inner.calls != 1 {
		t.Fatalf("normalized query should hit cache: calls=%d rs=%v", inner.calls, rs)
	}
	c.Search(ctx, "kosong", 1)
	c.Search(ctx, "kosong", 1)
	if inner.calls != 3 {
		t.Errorf("empty results must not be cached: calls=%d", inner.calls)
	}
	c.Search(ctx, "x", 1)
	c.Search(ctx, "y", 1) // melebihi max → cache dikosongkan, tetap berfungsi
	if rs, err := c.Search(ctx, "y", 1); err != nil || len(rs) != 1 {
		t.Errorf("after eviction: %v %v", rs, err)
	}
}

func TestRateLimitedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	old := braveHTMLURL
	braveHTMLURL = srv.URL
	defer func() { braveHTMLURL = old }()
	_, err := New(Config{}, srv.Client()).Search(context.Background(), "q", 3)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func TestChainFallsBackToBraveHTML(t *testing.T) {
	searx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer searx.Close()
	bh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<div data-type="web"><a href="https://b.test/x">` +
			`<div class="title" title="Cadangan">c</div></a><div class="content">isi</div></div>`))
	}))
	defer bh.Close()
	old := braveHTMLURL
	braveHTMLURL = bh.URL
	defer func() { braveHTMLURL = old }()
	s := New(Config{SearXNGURL: searx.URL}, bh.Client())
	rs, err := s.Search(context.Background(), "q", 3)
	if err != nil || len(rs) != 1 || rs[0].URL != "https://b.test/x" {
		t.Fatalf("fallback results = %+v, err %v", rs, err)
	}
	if s.Name() != "searxng" {
		t.Errorf("chain name = %s", s.Name())
	}
}
