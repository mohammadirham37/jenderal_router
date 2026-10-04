package api

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jenderal/jenderalrouter/internal/provider"
	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/translate"
	"github.com/jenderal/jenderalrouter/internal/usage"
)

// ---- Template provider (FR-1.1) ----

func (a *App) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"templates": store.ProviderTemplates})
}

// ---- Provider ----

func (a *App) handleListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := a.st.ListProviders()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	type pv struct {
		*store.Provider
		Credentials []store.PublicCredential `json:"credentials"`
		ModelCount  int                      `json:"model_count"`
	}
	out := []pv{}
	for _, p := range providers {
		creds, _ := a.st.ListCredentials(p.ID)
		pub := []store.PublicCredential{}
		for _, c := range creds {
			pub = append(pub, *c.Public())
		}
		models, _ := a.st.ListModels()
		count := 0
		for _, m := range models {
			if m.ProviderID == p.ID {
				count++
			}
		}
		out = append(out, pv{Provider: p, Credentials: pub, ModelCount: count})
	}
	writeJSON(w, 200, map[string]any{"providers": out})
}

func (a *App) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type     string                   `json:"type"`
		Name     string                   `json:"name"`
		Prefix   string                   `json:"prefix"`
		BaseURL  string                   `json:"base_url"`
		Settings store.CredentialSettings `json:"settings"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah: " + err.Error()})
		return
	}
	switch req.Type {
	case store.ProviderOpenAI, store.ProviderAnthropic, store.ProviderGemini,
		store.ProviderOpenAICompat, store.ProviderLlamaStash:
	default:
		writeJSON(w, 400, map[string]string{"error": "tipe provider tidak dikenal"})
		return
	}
	// SSRF guard untuk provider kustom (NFR-07)
	isLocal := req.Type == store.ProviderLlamaStash
	if err := provider.CheckBaseURL(req.BaseURL, isLocal, req.Settings.SSRFAllowPrivate); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	p, err := a.st.CreateProvider(req.Type, req.Name, req.Prefix, req.BaseURL, req.Settings, true)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "provider.create", "providers/"+fmtInt(p.ID), nil, p)
	writeJSON(w, 201, map[string]any{"provider": p})
}

func (a *App) handleSeedProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prefix   string `json:"prefix"`
		APIKey   string `json:"api_key"`
		Strategy string `json:"strategy"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	tpl := store.FindTemplate(req.Prefix)
	if tpl == nil {
		writeJSON(w, 404, map[string]string{"error": "template tidak ditemukan"})
		return
	}
	// idempoten: bila provider dengan prefix sudah ada, hanya tambah credential
	if existing, err := a.st.GetProviderByPrefix(req.Prefix); err == nil {
		if req.APIKey != "" {
			if _, err := a.st.AddCredential(existing.ID, "kunci", req.APIKey, 1); err != nil {
				writeJSON(w, 500, map[string]string{"error": err.Error()})
				return
			}
		}
		writeJSON(w, 200, map[string]any{"provider": existing, "seeded": false})
		return
	}
	p, err := a.st.SeedProvidersFromTemplate(tpl)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if req.APIKey != "" {
		if _, err := a.st.AddCredential(p.ID, "kunci", req.APIKey, 1); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
	}
	a.audit(r, "provider.seed", "providers/"+fmtInt(p.ID), nil, map[string]string{"template": req.Prefix})
	writeJSON(w, 201, map[string]any{"provider": p, "seeded": true})
}

func (a *App) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name     *string                   `json:"name"`
		BaseURL  *string                   `json:"base_url"`
		Settings *store.CredentialSettings `json:"settings"`
		Enabled  *bool                     `json:"enabled"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	if req.BaseURL != nil {
		p, err := a.st.GetProvider(id)
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": "provider tidak ada"})
			return
		}
		allow := false
		if req.Settings != nil {
			allow = req.Settings.SSRFAllowPrivate
		}
		if err := provider.CheckBaseURL(*req.BaseURL, p.Type == store.ProviderLlamaStash, allow); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
	}
	if err := a.st.UpdateProviderFields(id, req.Name, req.BaseURL, req.Settings, req.Enabled); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	p, _ := a.st.GetProvider(id)
	a.audit(r, "provider.update", "providers/"+fmtInt(id), nil, p)
	writeJSON(w, 200, map[string]any{"provider": p})
}

func (a *App) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.st.DeleteProvider(id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "provider.delete", "providers/"+fmtInt(id), nil, nil)
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

// handleTestProvider mengirim request kecil dan melaporkan latensi (FR-1.5).
func (a *App) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := a.st.GetProvider(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "provider tidak ada"})
		return
	}
	creds, err := a.st.ListCredentials(id)
	if err != nil || len(creds) == 0 {
		writeJSON(w, 400, map[string]string{"error": "provider belum punya credential"})
		return
	}
	secret, err := a.st.DecryptSecret(creds[0].SecretEnc)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "dekripsi secret gagal"})
		return
	}
	temp := 0.0
	mt := 8
	req := &translate.ChatRequest{
		Model:       a.pickTestModel(p, creds),
		Messages:    []translate.Message{{Role: translate.RoleUser, Content: []translate.Part{{Type: translate.PartText, Text: "ping"}}}},
		MaxTokens:   &mt,
		Temperature: &temp,
	}
	format := formatForProviderType(p.Type)
	if format == translate.FormatAnthropic && req.MaxTokens == nil {
		m := 8
		req.MaxTokens = &m
	}
	client := provider.NewClient()
	start := time.Now()
	ctx, cancel := contextWithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := client.Complete(ctx, provider.UpstreamReq{
		Format: format, BaseURL: p.BaseURL, APIKey: secret, Internal: req,
		Timeout: 30 * time.Second,
	})
	latency := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "latency_ms": latency, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "latency_ms": latency, "model": resp.Model,
		"sample": truncate(resp.Content, 80)})
}

// providerModelNames daftar nama upstream milik provider.
func (a *App) providerModelNames(providerID int64) []string {
	models, err := a.st.ListModels()
	if err != nil {
		return nil
	}
	var names []string
	for _, m := range models {
		if m.ProviderID == providerID {
			names = append(names, m.UpstreamName)
		}
	}
	return names
}

// pickTestModel memilih nama upstream pertama milik provider.
func (a *App) pickTestModel(p *store.Provider, creds []*store.Credential) string {
	models := a.providerModelNames(p.ID)
	if len(models) > 0 {
		return models[0]
	}
	// fallback generik per tipe
	switch p.Type {
	case store.ProviderAnthropic:
		return "claude-haiku-4-5"
	case store.ProviderGemini:
		return "gemini-2.5-flash-lite"
	default:
		return "gpt-4.1-mini"
	}
}

// handleSyncModels menyinkronkan daftar model dari provider (FR-1.6).
func (a *App) handleSyncModels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := a.st.GetProvider(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "provider tidak ada"})
		return
	}
	creds, _ := a.st.ListCredentials(id)
	var key string
	if len(creds) > 0 {
		key, _ = a.st.DecryptSecret(creds[0].SecretEnc)
	}
	names, source, err := a.fetchUpstreamModels(p, key)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error(), "source": source})
		return
	}
	added := 0
	for _, n := range names {
		if err := a.st.UpsertModelSinkron(p.ID, n, 0); err == nil {
			added++
		}
	}
	a.audit(r, "provider.sync_models", "providers/"+fmtInt(id), nil, map[string]int{"found": added})
	writeJSON(w, 200, map[string]any{"ok": true, "source": source, "models": names, "count": len(names)})
}

// fetchUpstreamModels mengambil daftar model upstream per format; LlamaStash
// juga mencoba CLI `llamastash list --json` (FR-6.2).
func (a *App) fetchUpstreamModels(p *store.Provider, key string) ([]string, string, error) {
	client := provider.NewClient()
	ctx, cancel := contextWithTimeout(nil, 15*time.Second)
	defer cancel()
	_ = ctx
	base := strings.TrimRight(p.BaseURL, "/")
	var url string
	switch p.Type {
	case store.ProviderAnthropic:
		url = base + "/v1/models"
	case store.ProviderGemini:
		url = base + "/models"
		if key != "" {
			url += "?key=" + key
		}
	default:
		url = base + "/models"
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "http", err
	}
	switch p.Type {
	case store.ProviderAnthropic:
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	case store.ProviderGemini:
	default:
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
	}
	resp, err := client.HTTP.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			names := parseModelList(p.Type, data)
			if len(names) > 0 {
				return names, "http", nil
			}
		}
	}
	// fallback CLI untuk LlamaStash
	if p.Type == store.ProviderLlamaStash {
		if names, err := llamastashCLIModels(); err == nil && len(names) > 0 {
			return names, "cli", nil
		}
	}
	if err != nil {
		return nil, "http", err
	}
	return nil, "http", fmt.Errorf("provider tidak mengembalikan daftar model")
}

// parseModelList menormalkan berbagai bentuk respons GET /models.
func parseModelList(ptype string, data []byte) []string {
	var openaiShape struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := jsonUnmarshalBytes(data, &openaiShape); err == nil && len(openaiShape.Data) > 0 {
		var out []string
		for _, m := range openaiShape.Data {
			if m.ID != "" {
				out = append(out, m.ID)
			}
		}
		return out
	}
	var idsShape struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := jsonUnmarshalBytes(data, &idsShape); err == nil && len(idsShape.Data) > 0 {
		var out []string
		for _, m := range idsShape.Data {
			n := strings.TrimPrefix(m.Name, "models/")
			if n != "" {
				out = append(out, n)
			}
		}
		return out
	}
	return nil
}

// llamastashCLIModels memanggil `llamastash list --json` bila binary ada.
func llamastashCLIModels() ([]string, error) {
	path := findLlamastashBin()
	if path == "" {
		return nil, fmt.Errorf("binary llamastash tidak ditemukan")
	}
	ctx := contextWithTimeoutCLI(10 * time.Second)
	out, err := exec.CommandContext(ctx, path, "list", "--json").Output()
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	if err := jsonUnmarshalBytes(out, &rows); err != nil {
		return nil, err
	}
	var names []string
	for _, row := range rows {
		n := row.Name
		if n == "" {
			n = row.ID
		}
		if n != "" {
			names = append(names, n)
		}
	}
	return names, nil
}

// ---- Credential ----

// handleCredentialSuggest GET /api/admin/providers/{id}/credentials/suggest —
// usul nilai API key untuk modal "Tambah API key": LlamaStash → bearer key
// daemon via CLI `llamastash api-key`; daemon keyless (loopback) → nilai
// acak yang diabaikan proxy. Provider vendor lain menolak — key-nya harus
// diterbitkan vendor.
func (a *App) handleCredentialSuggest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := a.st.GetProvider(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "provider tidak ditemukan"})
		return
	}
	if p.Type != store.ProviderLlamaStash {
		writeJSON(w, 400, map[string]string{"error": "key provider ini diterbitkan vendor-nya — tempel manual dari dashboard vendor"})
		return
	}
	bin := findLlamastashBin()
	if bin == "" {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "binary llamastash tidak ditemukan — isi key secara manual"})
		return
	}
	if key := llamastashAPIKey(bin); key != "" {
		writeJSON(w, 200, map[string]string{"api_key": key, "source": "cli"})
		return
	}
	writeJSON(w, 200, map[string]string{"api_key": generateLocalKey(), "source": "generated"})
}

// generateLocalKey nilai acak untuk daemon LlamaStash keyless — proxy lokal
// tidak memvalidasinya (stub keyless "llamastash" diabaikan).
func generateLocalKey() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("llama-%d", time.Now().UnixNano())
	}
	return "llama-" + hex.EncodeToString(b)
}

func (a *App) handleAddCredential(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Label  string `json:"label"`
		APIKey string `json:"api_key"`
		Weight int    `json:"weight"`
	}
	if err := decodeBody(w, r, &req); err != nil || req.APIKey == "" {
		writeJSON(w, 400, map[string]string{"error": "api_key wajib"})
		return
	}
	c, err := a.st.AddCredential(id, req.Label, req.APIKey, req.Weight)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "credential.create", "credentials/"+fmtInt(c.ID), nil, c.Public())
	writeJSON(w, 201, map[string]any{"credential": c.Public()})
}

func (a *App) handleDeleteCredential(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.st.DeleteCredential(id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "credential.delete", "credentials/"+fmtInt(id), nil, nil)
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

// ---- Model ----

func (a *App) handleListModels(w http.ResponseWriter, r *http.Request) {
	models, err := a.st.ListModels()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if models == nil {
		models = []*store.Model{}
	}
	writeJSON(w, 200, map[string]any{"models": models})
}

func (a *App) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Alias         *string         `json:"alias"`
		DisplayName   *string         `json:"display_name"`
		PriceInPer1M  *float64        `json:"price_in_per_1m"`
		PriceOutPer1M *float64        `json:"price_out_per_1m"`
		ContextWindow *int            `json:"context_window"`
		Capabilities  *store.ModelCap `json:"capabilities"`
		Enabled       *bool           `json:"enabled"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	if err := a.st.UpdateModelFields(id, req.Alias, req.DisplayName, req.PriceInPer1M, req.PriceOutPer1M,
		req.ContextWindow, req.Capabilities, req.Enabled); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	m, _ := a.st.GetModel(id)
	a.audit(r, "model.update", "models/"+fmtInt(id), nil, m)
	writeJSON(w, 200, map[string]any{"model": m})
}

// ---- Combo ----

func (a *App) handleListCombos(w http.ResponseWriter, r *http.Request) {
	combos, err := a.st.ListCombos()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if combos == nil {
		combos = []*store.Combo{}
	}
	writeJSON(w, 200, map[string]any{"combos": combos})
}

func (a *App) handleCreateCombo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		ModelIDs    []int64 `json:"model_ids"`
	}
	if err := decodeBody(w, r, &req); err != nil || req.Name == "" || len(req.ModelIDs) == 0 {
		writeJSON(w, 400, map[string]string{"error": "name dan model_ids wajib"})
		return
	}
	c, err := a.st.CreateCombo(req.Name, req.Description, req.ModelIDs)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "combo.create", "combos/"+fmtInt(c.ID), nil, c)
	writeJSON(w, 201, map[string]any{"combo": c})
}

func (a *App) handleUpdateCombo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		ModelIDs    []int64 `json:"model_ids"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	if err := a.st.UpdateCombo(id, req.Name, req.Description, req.ModelIDs); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	c, _ := a.st.GetCombo(id)
	a.audit(r, "combo.update", "combos/"+fmtInt(id), nil, c)
	writeJSON(w, 200, map[string]any{"combo": c})
}

func (a *App) handleDeleteCombo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.st.DeleteCombo(id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "combo.delete", "combos/"+fmtInt(id), nil, nil)
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

// ---- User & API key ----

func (a *App) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.st.ListUsers()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out := []map[string]any{}
	for _, u := range users {
		out = append(out, map[string]any{"user": u.Public()})
	}
	writeJSON(w, 200, map[string]any{"users": out})
}

// canManageUser RBAC: admin hanya boleh kelola non-admin (FR-4.1).
func canManageUser(actor *store.User, target *store.User) bool {
	if actor.Role == store.RoleSuperAdmin {
		return true
	}
	return target.Role == store.RoleMember || target.Role == store.RoleViewer
}

func (a *App) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	if req.Role == "" {
		req.Role = store.RoleMember
	}
	// hanya super_admin boleh membuat admin
	if req.Role == store.RoleAdmin || req.Role == store.RoleSuperAdmin {
		if ai.user.Role != store.RoleSuperAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "hanya super_admin boleh membuat admin"})
			return
		}
	}
	if err := cryptoStrength(req.Password); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	u, err := a.st.CreateUser(strings.TrimSpace(strings.ToLower(req.Email)), req.Password, req.Role)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "user.create", "users/"+fmtInt(u.ID), nil, u.Public())
	writeJSON(w, 201, map[string]any{"user": u.Public()})
}

func (a *App) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	target, err := a.st.GetUser(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "user tidak ada"})
		return
	}
	if !canManageUser(ai.user, target) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "tidak boleh mengelola user ini"})
		return
	}
	var req struct {
		Email          *string `json:"email"`
		Role           *string `json:"role"`
		Status         *string `json:"status"`
		Password       *string `json:"password"`
		DefaultComboID *int64  `json:"default_combo_id"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	if req.Role != nil && (*req.Role == store.RoleAdmin || *req.Role == store.RoleSuperAdmin) {
		if ai.user.Role != store.RoleSuperAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "hanya super_admin boleh mengangkat admin"})
			return
		}
	}
	if req.Password != nil {
		if err := cryptoStrength(*req.Password); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
	}
	if err := a.st.UpdateUserFields(id, req.Email, req.Role, req.Status, req.Password, req.DefaultComboID); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	u, _ := a.st.GetUser(id)
	a.audit(r, "user.update", "users/"+fmtInt(id), nil, u.Public())
	writeJSON(w, 200, map[string]any{"user": u.Public()})
}

func (a *App) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if id == ai.user.ID {
		writeJSON(w, 400, map[string]string{"error": "tidak bisa menghapus diri sendiri"})
		return
	}
	if err := a.st.DeleteUser(id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "user.delete", "users/"+fmtInt(id), nil, nil)
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

func (a *App) handleListKeys(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	keys, err := a.st.ListKeysByUser(id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if keys == nil {
		keys = []*store.APIKey{}
	}
	writeJSON(w, 200, map[string]any{"keys": keys})
}

// handleCreateKey membuat API key; plaintext hanya dikali ini (FR-4.2).
func (a *App) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name          string `json:"name"`
		AllowedModels string `json:"allowed_models"` // '*' atau daftar dipisah koma
		IPAllowlist   string `json:"ip_allowlist"`
		RPM           int    `json:"rpm"`
		TPM           int    `json:"tpm"`
		ExpiresAt     string `json:"expires_at"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	if req.AllowedModels == "" {
		req.AllowedModels = "*"
	}
	plain, hash, err := newAPIKey()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	k, err := a.st.CreateAPIKey(id, req.Name, plain, hash, req.AllowedModels, req.IPAllowlist, req.RPM, req.TPM, req.ExpiresAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.gate.InvalidateKey(hash)
	a.audit(r, "key.create", "keys/"+fmtInt(k.ID), nil, map[string]string{"prefix": k.Prefix})
	writeJSON(w, 201, map[string]any{"key": k, "plaintext": plain})
}

func (a *App) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	k, err := a.st.GetAPIKey(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "key tidak ada"})
		return
	}
	if err := a.st.DeleteAPIKey(id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.gate.InvalidateKey(k.KeyHash)
	a.audit(r, "key.delete", "keys/"+fmtInt(id), nil, nil)
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

// ---- Kuota ----

func (a *App) handlePutUserQuota(w http.ResponseWriter, r *http.Request) {
	a.putQuota(w, r, "user")
}

func (a *App) handlePutKeyQuota(w http.ResponseWriter, r *http.Request) {
	a.putQuota(w, r, "key")
}

func (a *App) putQuota(w http.ResponseWriter, r *http.Request, scope string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Period       string  `json:"period"` // day|month
		TokenLimit   int64   `json:"token_limit"`
		RequestLimit int64   `json:"request_limit"`
		CostLimitUSD float64 `json:"cost_limit_usd"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	if req.Period != "day" && req.Period != "month" {
		writeJSON(w, 400, map[string]string{"error": "period harus day atau month"})
		return
	}
	q, err := a.st.PutQuota(scope, id, req.Period, req.TokenLimit, req.RequestLimit, req.CostLimitUSD)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "quota.put", scope+"s/"+fmtInt(id), nil, q)
	writeJSON(w, 200, map[string]any{"quota": q})
}

// ---- Log & analitik ----

func (a *App) handleListLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from := q.Get("from")
	to := q.Get("to")
	if from == "" {
		from = time.Now().AddDate(0, 0, -7).UTC().Format(time.RFC3339)
	}
	if to == "" {
		to = time.Now().UTC().Format(time.RFC3339)
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	logs, err := a.queryLogs(from, to, q.Get("user"), q.Get("provider"), q.Get("status"), limit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"logs": logs})
}

// handleExportLogs mengekspor log ke CSV (FR-5.4).
func (a *App) handleExportLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from := q.Get("from")
	to := q.Get("to")
	if from == "" {
		from = time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339)
	}
	if to == "" {
		to = time.Now().UTC().Format(time.RFC3339)
	}
	logs, err := a.queryLogs(from, to, q.Get("user"), q.Get("provider"), q.Get("status"), 100000)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=jenderalrouter-logs.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"ts", "user_id", "key_id", "requested_model", "provider", "model",
		"status", "error_code", "latency_ms", "ttft_ms", "tokens_in", "tokens_out", "cost_usd", "attempts"})
	for _, l := range logs {
		_ = cw.Write([]string{
			l["ts"], l["user_id"], l["key_id"], l["requested_model"], l["provider_name"], l["model_name"],
			l["status"], l["error_code"], l["latency_ms"], l["ttft_ms"], l["tokens_in"], l["tokens_out"],
			l["cost_usd"], l["attempts"],
		})
	}
	cw.Flush()
}

func (a *App) handleAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	days, _ := strconv.Atoi(q.Get("days"))
	if days <= 0 || days > 365 {
		days = 7
	}
	now := time.Now()
	from := now.AddDate(0, 0, -days)
	res, err := usage.GetAnalytics(a.st, from, now.Add(time.Minute))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

func (a *App) handleListAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := a.st.ListAudit(limit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"audit": entries})
}

// ---- Settings ----

func (a *App) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{}
	for _, key := range []string{"notification_telegram_token", "notification_chat_id", "public_url"} {
		v, _ := a.st.GetSetting(key)
		out[key] = v
	}
	out["log_prompts"] = boolStr(a.cfg.LogPrompts)
	out["tz"] = a.cfg.TZ
	writeJSON(w, 200, map[string]any{"settings": out})
}

func (a *App) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	for k, v := range req {
		if strings.Contains(k, "secret") || strings.Contains(k, "token") {
			// nilai sensitif: tidak boleh dikembalikan via GET, disimpan saja
		}
		if err := a.st.SetSetting(k, v); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
	}
	a.audit(r, "settings.update", "settings", nil, req)
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

// ---- Sistem ----

func (a *App) handleSystemBackup(w http.ResponseWriter, r *http.Request) {
	if err := a.backupDatabase(); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "system.backup", "system", nil, nil)
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

// ---- LlamaStash (FR-6.3) ----

func (a *App) handleLocalStatus(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"daemon_alive": false, "models": []any{}, "url": a.cfg.LLamastashURL}
	base := strings.TrimRight(a.cfg.LLamastashURL, "/")
	origin := strings.TrimSuffix(base, "/v1") // endpoint /health & /v1/* hidup di origin
	client := provider.NewClient()
	var bearer string
	if p, err := a.st.GetProviderByPrefix("local"); err == nil {
		if creds, _ := a.st.ListCredentials(p.ID); len(creds) > 0 {
			bearer, _ = a.st.DecryptSecret(creds[0].SecretEnc)
		}
	}
	doGet := func(path string) (int, []byte) {
		req, _ := http.NewRequest(http.MethodGet, origin+path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		start := time.Now()
		resp, err := client.HTTP.Do(req)
		if err != nil {
			return 0, nil
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if path == "/health" {
			out["latency_ms"] = time.Since(start).Milliseconds()
		}
		return resp.StatusCode, data
	}

	// /health — probe resmi yang selalu terbuka: {"status","models_loaded","models_discovered"}
	if code, data := doGet("/health"); code > 0 && code < 500 {
		out["daemon_alive"] = true
		var h struct {
			Status           string `json:"status"`
			ModelsLoaded     int    `json:"models_loaded"`
			ModelsDiscovered int    `json:"models_discovered"`
		}
		if json.Unmarshal(data, &h) == nil {
			out["models_loaded"] = h.ModelsLoaded
			out["models_discovered"] = h.ModelsDiscovered
		}
	}

	// /v1/models — daftar model yang ditemukan (termasuk yang belum dimuat)
	var runningNames []string
	if rm, ok := out["running_models"].([]string); ok {
		runningNames = rm
	}
	if code, data := doGet("/v1/models"); code == 200 {
		names := parseModelList("openai", data)
		models := []map[string]any{}
		for _, n := range names {
			loaded := false
			for _, rn := range runningNames {
				if strings.Contains(strings.ToLower(n), strings.ToLower(rn)) || strings.Contains(strings.ToLower(rn), strings.ToLower(n)) {
					loaded = true
					break
				}
			}
			models = append(models, map[string]any{"name": n, "loaded": loaded})
		}
		out["models"] = models
	}
	// coba CLI untuk info tambahan (bila binary tersedia di host)
	if names, err := llamastashCLIModels(); err == nil {
		out["cli_available"] = true
		existing, _ := out["models"].([]map[string]any)
		known := map[string]bool{}
		for _, m := range existing {
			known[m["name"].(string)] = true
		}
		for _, n := range names {
			if !known[n] {
				existing = append(existing, map[string]any{"name": n, "loaded": false})
			}
		}
		out["models"] = existing
	} else {
		out["cli_available"] = false
	}
	// metadata instalasi untuk tombol Install di dashboard (FR-6.8)
	for k, v := range localStatusExtra(a.cfg.LLamastashURL) {
		out[k] = v
	}
	writeJSON(w, 200, out)
}

func (a *App) handleLocalModelStart(w http.ResponseWriter, r *http.Request) {
	a.localModelToggle(w, r, "start")
}

func (a *App) handleLocalModelStop(w http.ResponseWriter, r *http.Request) {
	a.localModelToggle(w, r, "stop")
}

// localModelToggle menjalankan perintah CLI llamastash start/stop (FR-6.3).
func (a *App) localModelToggle(w http.ResponseWriter, r *http.Request, action string) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, 400, map[string]string{"error": "nama model wajib"})
		return
	}
	path := findLlamastashBin()
	if path == "" {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "CLI llamastash tidak tersedia di dalam proses ini; jalankan '" + action + " " + name +
				"' di host, atau pasang binary llamastash agar tombol ini aktif"})
		return
	}
	var out []byte
	var err error
	if action == "start" {
		// start --json → {name, launch_id, port, pid, preset, path} (v0.6.1);
		// load model bisa lama (probe timeout daemon 120 dtk)
		c2 := contextWithTimeoutCLI(240 * time.Second)
		out, err = exec.CommandContext(c2, path, "start", name, "--json").Output()
	} else {
		c2 := contextWithTimeoutCLI(60 * time.Second)
		out, err = exec.CommandContext(c2, path, "stop", name).Output()
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error(), "output": string(out)})
		return
	}
	a.audit(r, "local.model."+action, "local/models/"+name, nil, nil)
	writeJSON(w, 200, map[string]string{"ok": "true", "output": string(out)})
}

// ---- util ----

func (a *App) audit(r *http.Request, action, target string, before, after any) {
	if ai := authFrom(r); ai != nil {
		_ = a.st.Audit(ai.user.ID, action, target, before, after)
	}
}

// pathID mengambil {id} dari path.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, 400, map[string]string{"error": "id tidak sah"})
		return 0, false
	}
	return id, true
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func formatForProviderType(ptype string) translate.Format {
	switch ptype {
	case store.ProviderAnthropic:
		return translate.FormatAnthropic
	case store.ProviderGemini:
		return translate.FormatGemini
	default:
		return translate.FormatOpenAI
	}
}
