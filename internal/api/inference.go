package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jenderal/jenderalrouter/internal/provider"
	"github.com/jenderal/jenderalrouter/internal/router"
	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/translate"
)

// ===== Resolver: nama request → langkah combo (FR-2.1) =====

// storeResolver implementasi router.Resolver di atas store.
type storeResolver struct{ st *store.Store }

// Resolve memetakan nama ke 1..n langkah: combo → banyak langkah;
// alias/public_id → satu langkah.
func (r storeResolver) Resolve(name string) ([]router.Step, error) {
	if combo, err := r.st.GetComboByName(name); err == nil {
		var steps []router.Step
		for _, cs := range combo.Steps {
			m, err := r.st.GetModel(cs.ModelID)
			if err != nil {
				continue
			}
			if !m.Enabled || !m.ProviderEnabled {
				continue
			}
			steps = append(steps, modelToStep(m, cs.TimeoutMs))
		}
		if len(steps) == 0 {
			return nil, fmt.Errorf("combo %q tidak punya langkah aktif", name)
		}
		return steps, nil
	}
	m, err := r.st.FindModelByPublicOrAlias(name)
	if err != nil {
		return nil, fmt.Errorf("model/combo %q tidak ditemukan", name)
	}
	if !m.Enabled || !m.ProviderEnabled {
		return nil, fmt.Errorf("model %q sedang nonaktif", name)
	}
	return []router.Step{modelToStep(m, 0)}, nil
}

// modelToStep memetakan model store ke langkah router termasuk format wire.
func modelToStep(m *store.Model, timeoutMs int) router.Step {
	if timeoutMs <= 0 {
		timeoutMs = 120000
	}
	return router.Step{
		ModelID:    m.ID,
		ProviderID: m.ProviderID,
		PublicID:   m.PublicID,
		Upstream:   m.UpstreamName,
		Format:     formatForProvider(m.ProviderType),
		TimeoutMs:  timeoutMs,
	}
}

func formatForProvider(ptype string) translate.Format {
	switch ptype {
	case store.ProviderAnthropic:
		return translate.FormatAnthropic
	case store.ProviderGemini:
		return translate.FormatGemini
	default: // openai, openai-compatible, llamastash
		return translate.FormatOpenAI
	}
}

// ===== Executor: eksekusi langkah + credential (FR-2.4, FR-1.4) =====

// stepExecutor mengeksekusi langkah-langkah satu request inferensi: memilih
// credential (menghindari credential yang sudah gagal di request yang sama),
// menandai sukses/kegagalan credential, dan menahan konkurensi lokal.
type stepExecutor struct {
	app      *App
	internal *translate.ChatRequest // salinan request dgn nama upstream per langkah
	stream   bool

	mu   sync.Mutex
	used map[int64]map[int64]bool // modelID → credentialID yang sudah dicoba
}

func newStepExecutor(app *App, internal *translate.ChatRequest, stream bool) *stepExecutor {
	return &stepExecutor{app: app, internal: internal, stream: stream, used: map[int64]map[int64]bool{}}
}

func (se *stepExecutor) markUsed(modelID, credID int64) {
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.used[modelID] == nil {
		se.used[modelID] = map[int64]bool{}
	}
	se.used[modelID][credID] = true
}

func (se *stepExecutor) usedSet(modelID int64) map[int64]bool {
	se.mu.Lock()
	defer se.mu.Unlock()
	out := map[int64]bool{}
	for k := range se.used[modelID] {
		out[k] = true
	}
	return out
}

// send menjalankan satu langkah (dipakai router.Runner sebagai Sender).
func (se *stepExecutor) send(ctx context.Context, step router.Step) (*router.AttemptResult, error) {
	sr := &stepRun{app: se.app, step: step, stream: se.stream, skipCred: se.usedSet(step.ModelID)}
	res, err := sr.do(ctx, se.internal)
	if err != nil {
		return nil, err
	}
	se.markUsed(step.ModelID, res.CredentialID)
	return res, nil
}

// stepRun eksekusi tunggal (satu credential) atas satu langkah.
type stepRun struct {
	app      *App
	step     router.Step
	stream   bool
	skipCred map[int64]bool

	prov     *store.Provider
	settings store.CredentialSettings
	cred     *store.Credential
}

// prepare memuat provider, settings, SSRF guard, dan memilih credential.
func (sr *stepRun) prepare() error {
	p, err := sr.app.st.GetProvider(sr.step.ProviderID)
	if err != nil {
		return &translate.UpstreamError{StatusCode: 0, Message: "provider tidak ditemukan"}
	}
	if !p.Enabled {
		return &translate.UpstreamError{StatusCode: 0, Message: "provider dinonaktifkan"}
	}
	sr.prov = p
	if len(p.Settings) > 0 {
		if err := json.Unmarshal(p.Settings, &sr.settings); err != nil {
			return &translate.UpstreamError{StatusCode: 0, Message: "settings provider rusak"}
		}
	}
	isLocal := p.Type == store.ProviderLlamaStash
	if err := provider.CheckBaseURL(p.BaseURL, isLocal, sr.settings.SSRFAllowPrivate); err != nil {
		return &translate.UpstreamError{StatusCode: 0, Message: err.Error()}
	}
	cred, err := sr.app.st.PickCredential(p.ID, sr.settings.Strategy, sr.skipCred)
	if err != nil {
		return &translate.UpstreamError{StatusCode: 0, Message: err.Error()}
	}
	sr.cred = cred
	return nil
}

// do menjalankan panggilan upstream (termasuk antrean konkurensi lokal FR-6.5).
func (sr *stepRun) do(ctx context.Context, internal *translate.ChatRequest) (*router.AttemptResult, error) {
	if err := sr.prepare(); err != nil {
		return nil, err
	}
	if sr.prov.Type == store.ProviderLlamaStash {
		release, err := sr.app.localGate.acquire(sr.step.ModelID, sr.settings, ctx)
		if err != nil {
			return nil, &translate.UpstreamError{StatusCode: 429, Message: "antrean model lokal penuh: " + err.Error()}
		}
		defer release()
	}
	secret, err := sr.app.st.DecryptSecret(sr.cred.SecretEnc)
	if err != nil {
		return nil, &translate.UpstreamError{StatusCode: 0, Message: "dekripsi secret gagal"}
	}

	ureq := provider.UpstreamReq{
		Format:   sr.step.Format,
		BaseURL:  sr.prov.BaseURL,
		APIKey:   secret,
		Headers:  sr.settings.ExtraHeaders,
		Internal: sr.upstreamRequest(internal),
		Stream:   sr.stream,
		Timeout:  sr.timeout(),
	}

	if sr.stream {
		es, err := sr.app.client.Stream(ctx, ureq)
		if err != nil {
			sr.markFailure(err)
			return nil, err
		}
		return &router.AttemptResult{
			ProviderID: sr.prov.ID, ProviderName: sr.prov.Name,
			ModelPublic: sr.step.PublicID, CredentialID: sr.cred.ID, Stream: es,
		}, nil
	}
	resp, err := sr.app.client.Complete(ctx, ureq)
	if err != nil {
		sr.markFailure(err)
		return nil, err
	}
	// FR-2.2: konten kosong dianggap gagal dan layak fallback
	if resp.IsEmpty() {
		err := &translate.UpstreamError{StatusCode: 200, Message: "provider mengembalikan konten kosong"}
		sr.markFailure(err)
		return nil, err
	}
	sr.app.st.MarkCredentialSuccess(sr.cred.ID)
	return &router.AttemptResult{
		ProviderID: sr.prov.ID, ProviderName: sr.prov.Name,
		ModelPublic: sr.step.PublicID, CredentialID: sr.cred.ID, Response: resp,
	}, nil
}

// upstreamRequest menyalin request internal dengan nama model upstream.
func (sr *stepRun) upstreamRequest(internal *translate.ChatRequest) *translate.ChatRequest {
	cp := *internal
	cp.Model = sr.step.Upstream
	if cp.MaxTokens == nil && sr.step.Format == translate.FormatAnthropic {
		mt := 4096 // Anthropic mewajibkan max_tokens
		cp.MaxTokens = &mt
	}
	return &cp
}

// timeout batas waktu langkah; lokal menghormati cold start 120 dtk (FR-6.4).
func (sr *stepRun) timeout() time.Duration {
	if sr.prov != nil && sr.prov.Type == store.ProviderLlamaStash {
		if sr.settings.ColdStartTimeoutMs > 0 {
			return time.Duration(sr.settings.ColdStartTimeoutMs) * time.Millisecond
		}
		return 120 * time.Second
	}
	if sr.step.TimeoutMs > 0 {
		return time.Duration(sr.step.TimeoutMs) * time.Millisecond
	}
	return 120 * time.Second
}

// markFailure mencatat kegagalan credential (cooldown FR-1.4).
func (sr *stepRun) markFailure(err error) {
	status := 0
	if ue, ok := err.(*translate.UpstreamError); ok {
		status = ue.StatusCode
	}
	_ = sr.app.st.MarkCredentialFailure(sr.cred.ID, status, err.Error())
}

// ===== LocalGate: konkurensi & antrean model lokal (FR-6.5) =====

// localGate semaphore per model untuk provider LlamaStash.
type localGate struct {
	mu    sync.Mutex
	slots map[int64]chan struct{}
	limit int
	queue time.Duration
}

func newLocalGate() *localGate {
	return &localGate{slots: map[int64]chan struct{}{}, limit: 2, queue: 30 * time.Second}
}

func (g *localGate) configure(s store.CredentialSettings) {
	if s.LocalConcurrency > 0 {
		g.limit = s.LocalConcurrency
	}
	if s.LocalQueueTimeout > 0 {
		g.queue = time.Duration(s.LocalQueueTimeout) * time.Millisecond
	}
}

// acquire menunggu slot hingga batas antrean; ctx cancel membatalkan.
func (g *localGate) acquire(modelID int64, s store.CredentialSettings, ctx context.Context) (func(), error) {
	g.mu.Lock()
	g.configure(s)
	ch, ok := g.slots[modelID]
	if !ok {
		ch = make(chan struct{}, g.limit)
		g.slots[modelID] = ch
	}
	g.mu.Unlock()
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	case <-time.After(g.queue):
		return nil, fmt.Errorf("menunggu lebih dari %s", g.queue)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
