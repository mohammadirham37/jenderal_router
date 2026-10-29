// Package web menyajikan UI SvelteKit yang sudah di-build (embed ke binary).
// SPA fallback: path non-file dilayani index.html (routing client-side).
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
)

var inlineScriptRe = regexp.MustCompile(`(?s)<script>(.*?)</script>`)

// CSP kebijakan keamanan konten untuk UI. Script inline bawaan SvelteKit
// (bootstrap + tema) diizinkan lewat hash SHA-256 yang dihitung dari
// index.html ter-embed — tanpa 'unsafe-inline' untuk script. Style inline
// dipakai luas oleh komponen, jadi style-src mengizinkannya.
var CSP = func() string {
	hashes := ""
	if b, err := distFS.ReadFile("dist/index.html"); err == nil {
		for _, m := range inlineScriptRe.FindAllSubmatch(b, -1) {
			sum := sha256.Sum256(m[1])
			hashes += " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		}
	}
	return "default-src 'self'; script-src 'self'" + hashes +
		"; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:" +
		"; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
}()

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
				if strings.HasPrefix(path, "_app/immutable/") {
					// nama file ber-hash konten → aman di-cache permanen
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
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
