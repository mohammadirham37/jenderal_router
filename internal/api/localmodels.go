package api

// Unduh model LlamaStash dari dashboard: daftar rekomendasi hardware-aware
// via `llamastash recommend --json` (read-only, kontrak resmi v0.6.1), lalu
// unduh model pilihan via `llamastash pull owner/repo[:file.gguf]` sebagai
// job asinkron dengan log live — pola yang sama dengan job install.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// hfAPIBase basis API publik HuggingFace (dioverride di test).
var hfAPIBase = "https://huggingface.co"

var hfHTTPClient = &http.Client{Timeout: 15 * time.Second}

// hfListFiles daftar file .gguf di sebuah repo HF; (nil, nil) bila repo
// tidak ada / tidak memuat GGUF.
func hfListFiles(ctx context.Context, repo string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hfAPIBase+"/api/models/"+repo, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hfHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hf %s: status %d", repo, resp.StatusCode)
	}
	var m struct {
		Siblings []struct {
			RFilename string `json:"rfilename"`
		} `json:"siblings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	var ggufs []string
	for _, s := range m.Siblings {
		n := strings.ToLower(s.RFilename)
		if strings.HasSuffix(n, ".gguf") && !strings.Contains(n, "mmproj") {
			ggufs = append(ggufs, s.RFilename)
		}
	}
	return ggufs, nil
}

// hfSearchRepos cari repo GGUF di HF, terurut unduhan terbanyak.
func hfSearchRepos(ctx context.Context, query string) ([]string, error) {
	u := hfAPIBase + "/api/models?search=" + url.QueryEscape(query) +
		"&limit=5&sort=downloads&direction=-1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hfHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hf search: status %d", resp.StatusCode)
	}
	var hits []struct {
		ModelID string `json:"modelId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&hits); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		if h.ModelID != "" {
			ids = append(ids, h.ModelID)
		}
	}
	return ids, nil
}

// normQuant bentuk baku quant untuk pencocokan: "Q6_K" → "q6k".
func normQuant(q string) string {
	q = strings.ToLower(q)
	for _, c := range []string{"-", "_", "."} {
		q = strings.ReplaceAll(q, c, "")
	}
	return q
}

// pickGGUFPinned pilih satu file GGUF yang memuat quant; yang terpendek
// menang agar varian biasa (Q6_K) diutamakan di atas varian berlabel
// (UD-Q6_K_XL, dsb.).
func pickGGUFPinned(files []string, quant string) string {
	q := normQuant(quant)
	if q == "" {
		return ""
	}
	var best string
	for _, f := range files {
		if strings.Contains(normQuant(f), q) && (best == "" || len(f) < len(best)) {
			best = f
		}
	}
	return best
}

// resolveGGUFRepo temukan repo+file GGUF asli untuk entri katalog
// rekomendasi: nama file-nya sintetis (gguf_publisher "synthetic"), kuant
// aslinya diterbitkan repo lain (mis. Qwen/*-GGUF, unsloth/*).
// Urutan: konvensi <repo>-GGUF → pencarian HF "<basename> gguf".
func resolveGGUFRepo(ctx context.Context, repo, quant string) (string, string, error) {
	q := normQuant(quant)
	if f := pickGGUFPinned(mustFiles(ctx, repo+"-GGUF"), quant); f != "" {
		return repo + "-GGUF", f, nil
	}
	base := repo
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	hits, err := hfSearchRepos(ctx, base+" gguf")
	if err == nil {
		for _, cand := range hits {
			if strings.EqualFold(cand, repo) {
				continue // repo sumber (safetensors) — bukan yang dicari
			}
			files, err := hfListFiles(ctx, cand)
			if err != nil {
				continue
			}
			if f := pickGGUFPinned(files, quant); f != "" {
				return cand, f, nil
			}
		}
	}
	_ = q
	return "", "", fmt.Errorf(
		"tidak menemukan GGUF %s untuk %s — kuantisasi ini diterbitkan pihak ketiga; cari di Hugging Face (mis. unsloth/<model>-GGUF) lalu tempel di input repo kustom",
		quant, repo)
}

func mustFiles(ctx context.Context, repo string) []string {
	files, err := hfListFiles(ctx, repo)
	if err != nil {
		return nil
	}
	return files
}

// ---- rekomendasi (kontrak `recommend --json`: bentuk sama dengan
// `init --only models --json`) ----

func (a *App) handleLocalModelRecommendations(w http.ResponseWriter, r *http.Request) {
	bin := findLlamastashBin()
	if bin == "" {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "binary llamastash tidak ditemukan — install dulu dari halaman ini"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	// --model none = dry-run resmi: TANPA mengunduh model. `recommend`
	// bawaannya langsung mengunduh pick teratas (diverifikasi thd v0.6.1)!
	out, err := exec.CommandContext(ctx, bin, "recommend", "--model", "none", "--json").Output()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "recommend gagal: " + err.Error()})
		return
	}
	var parsed struct {
		Hardware struct {
			GPUBackend    string `json:"gpu_backend"`
			RAMTotalBytes int64  `json:"ram_total_bytes"`
		} `json:"hardware"`
		Recommendations []struct {
			Kind struct {
				Type  string `json:"type"`
				Entry struct {
					ID            string `json:"id"`
					Repo          string `json:"repo"`
					File          string `json:"file"`
					Quant         string `json:"quant"`
					WeightsBytes  int64  `json:"weights_bytes"`
					IsMoE         bool   `json:"is_moe"`
					ParamsActive  *int64 `json:"params_active"`
					GGUFPublisher string `json:"gguf_publisher"`
					Benchmark     struct {
						Value float64 `json:"value"`
					} `json:"benchmark_score"`
				} `json:"entry"`
			} `json:"kind"`
			Justification      string `json:"justification"`
			EstimatedPeakBytes *int64 `json:"estimated_peak_bytes"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		writeJSON(w, 500, map[string]string{"error": "output recommend tidak sah: " + err.Error()})
		return
	}
	recs := []map[string]any{}
	for _, rc := range parsed.Recommendations {
		if rc.Kind.Type != "curated" {
			continue // lewati entri "escape" (paste HF repo id)
		}
		e := rc.Kind.Entry
		m := map[string]any{
			"id":            e.ID,
			"repo":          e.Repo,
			"file":          e.File,
			"quant":         e.Quant,
			"weights_gb":    round2(float64(e.WeightsBytes) / 1e9),
			"bench":         e.Benchmark.Value,
			"moe":           e.IsMoE,
			"synthetic":     e.GGUFPublisher == "synthetic",
			"justification": rc.Justification,
		}
		if e.ParamsActive != nil {
			m["params_active_b"] = *e.ParamsActive
		}
		if rc.EstimatedPeakBytes != nil {
			m["peak_gb"] = round2(float64(*rc.EstimatedPeakBytes) / 1e9)
		}
		recs = append(recs, m)
	}
	writeJSON(w, 200, map[string]any{
		"recommendations": recs,
		"hardware": map[string]any{
			"gpu_backend":  parsed.Hardware.GPUBackend,
			"ram_total_gb": round2(float64(parsed.Hardware.RAMTotalBytes) / 1e9),
		},
	})
}

// ---- job unduh (reuse installJob: fase + log live + snapshot) ----

var modelDlState struct {
	mu  sync.Mutex
	job *installJob
}

// owner/repo atau owner/repo:file.gguf — HF slug + nama file GGUF.
var hfSpecRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(:[A-Za-z0-9_.-]+)?$`)

func (a *App) handleLocalModelDownload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repo    string `json:"repo"`
		File    string `json:"file"`
		Quant   string `json:"quant"`
		Resolve bool   `json:"resolve"` // entri katalog: nama file sintetis → cari GGUF asli
	}
	if err := decodeBody(w, r, &req); err != nil || req.Repo == "" {
		writeJSON(w, 400, map[string]string{"error": "repo wajib (owner/repo[:file.gguf])"})
		return
	}
	spec := req.Repo
	if req.File != "" {
		spec = req.Repo + ":" + req.File
	}
	if !hfSpecRe.MatchString(spec) {
		writeJSON(w, 400, map[string]string{"error": "format tidak sah — pakai owner/repo atau owner/repo:file.gguf"})
		return
	}
	bin := findLlamastashBin()
	if bin == "" {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "binary llamastash tidak ditemukan"})
		return
	}
	// resolusi sinkron (butuh jawaban gagal yang jelas bila tidak ketemu);
	// beberapa panggilan HF publik, puluhan ms masing-masing
	resolved := ""
	if req.Resolve {
		ggufRepo, file, err := resolveGGUFRepo(r.Context(), req.Repo, req.Quant)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		spec = ggufRepo + ":" + file
		resolved = spec
	}
	modelDlState.mu.Lock()
	if modelDlState.job != nil && modelDlState.job.running {
		modelDlState.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "unduhan lain sedang berjalan — pantau lognya"})
		return
	}
	job := &installJob{running: true, phase: "menyiapkan unduhan", started: time.Now()}
	modelDlState.job = job
	modelDlState.mu.Unlock()

	ai := authFrom(r)
	actorID := int64(0)
	if ai != nil {
		actorID = ai.user.ID
	}

	go func() {
		// unduhan model bisa sangat besar (puluhan GB) — batas longgar
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
		defer cancel()
		if resolved != "" {
			job.appendLog("resolve: %s:%s → %s", req.Repo, req.Quant, resolved)
		}
		job.setPhase("mengunduh " + spec + " (bisa sangat lama, tergantung koneksi)")
		if err := job.runStreamed(ctx, bin, "pull", spec); err != nil {
			job.fail("pull gagal: " + err.Error())
			a.st.Audit(actorID, "local.model.download", "llamastash/"+spec, nil, map[string]any{"ok": false})
			return
		}
		job.succeed()
		a.st.Audit(actorID, "local.model.download", "llamastash/"+spec, nil, map[string]any{"ok": true})
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"started": true, "status_url": "/api/admin/local/models/download/status"})
}

func (a *App) handleLocalModelDownloadStatus(w http.ResponseWriter, r *http.Request) {
	modelDlState.mu.Lock()
	job := modelDlState.job
	modelDlState.mu.Unlock()
	if job == nil {
		writeJSON(w, http.StatusOK, installJobSnapshot{Running: false, Done: false, Log: []string{}})
		return
	}
	writeJSON(w, http.StatusOK, job.snapshot())
}
