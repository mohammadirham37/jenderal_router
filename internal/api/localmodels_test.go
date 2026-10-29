package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubHF server HuggingFace palsu: konvensi <repo>-GGUF + hasil pencarian.
func stubHF(t *testing.T) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/Qwen/Qwen3-30B-A3B-GGUF", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"siblings":[{"rfilename":"Qwen3-30B-A3B-Q6_K.gguf"},{"rfilename":"Qwen3-30B-A3B-Q4_K_M.gguf"}]}`))
	})
	mux.HandleFunc("/api/models/unsloth/gpt-oss-20b-GGUF", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"siblings":[{"rfilename":"gpt-oss-20b-UD-Q6_K_XL.gguf"},{"rfilename":"gpt-oss-20b-Q6_K.gguf"},{"rfilename":"mmproj-gpt-oss-20b-Q6_K.gguf"}]}`))
	})
	mux.HandleFunc("/api/models", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("search"), "gpt-oss") {
			w.Write([]byte(`[{"modelId":"openai/gpt-oss-20b"},{"modelId":"unsloth/gpt-oss-20b-GGUF"}]`))
			return
		}
		w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	old := hfAPIBase
	hfAPIBase = srv.URL
	t.Cleanup(func() { hfAPIBase = old })
}

func TestResolveGGUFDirectConvention(t *testing.T) {
	stubHF(t)
	repo, file, err := resolveGGUFRepo(context.Background(), "Qwen/Qwen3-30B-A3B", "Q6_K")
	if err != nil {
		t.Fatalf("resolve gagal: %v", err)
	}
	if repo != "Qwen/Qwen3-30B-A3B-GGUF" || file != "Qwen3-30B-A3B-Q6_K.gguf" {
		t.Fatalf("hasil: %s:%s", repo, file)
	}
}

func TestResolveGGUFSearchFallback(t *testing.T) {
	stubHF(t)
	repo, file, err := resolveGGUFRepo(context.Background(), "openai/gpt-oss-20b", "Q6_K")
	if err != nil {
		t.Fatalf("resolve gagal: %v", err)
	}
	if repo != "unsloth/gpt-oss-20b-GGUF" {
		t.Fatalf("repo: %s", repo)
	}
	// varian berlabel (UD-XL, mmproj) tak boleh menang atas Q6_K polos
	if file != "gpt-oss-20b-Q6_K.gguf" {
		t.Fatalf("file: %s", file)
	}
}

func TestResolveGGUFTidakKetemu(t *testing.T) {
	stubHF(t)
	_, _, err := resolveGGUFRepo(context.Background(), "google/gemma-4-31B-it", "Q6_K")
	if err == nil {
		t.Fatal("harusnya gagal")
	}
	if !strings.Contains(err.Error(), "pihak ketiga") {
		t.Fatalf("pesan tidak membimbing: %v", err)
	}
}

// argumen ke CLI llamastash tidak boleh bisa dibaca sebagai flag / traversal.
func TestLlamastashArgValidation(t *testing.T) {
	for spec, ok := range map[string]bool{
		"unsloth/Qwen3-30B-A3B-GGUF": true, "owner/repo:Q6_K.gguf": true,
		"-o/tmp": false, "--x/y": false, "../..:..": false, "owner/-repo": false,
		"owner/repo:-f": false, "a/b..c": false,
	} {
		if got := validHFSpec(spec); got != ok {
			t.Errorf("validHFSpec(%q) = %v", spec, got)
		}
	}
	for name, ok := range map[string]bool{
		"Qwen3-30B-A3B-Q6_K": true, "org/model:q4": true,
		"--config=x": false, "-h": false, "../etc/passwd": false, "": false,
	} {
		if got := validLocalModelName(name); got != ok {
			t.Errorf("validLocalModelName(%q) = %v", name, got)
		}
	}
}
