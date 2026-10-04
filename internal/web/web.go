// Package web menyajikan UI SvelteKit yang sudah di-build (embed ke binary).
// SPA fallback: path non-file dilayani index.html (routing client-side).
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler melayani UI statis dengan fallback SPA.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		// aset build (/_app/…) dan file statis dilayani langsung
		if path != "" {
			if _, err := fs.Stat(sub, path); err == nil && !strings.HasSuffix(path, "/") {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// sisanya → index.html (SPA)
		index, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("UI belum ter-embed (build ulang binary)"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(index)
	})
}
