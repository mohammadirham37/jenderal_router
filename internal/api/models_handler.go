package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jenderal/jenderalrouter/internal/apigate"
	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/translate"
	"github.com/jenderal/jenderalrouter/internal/usage"
)

// modelsInference menangani GET /v1/models: daftar model + combo yang
// diizinkan untuk key ini (F-03).
func (a *App) modelsInference(w http.ResponseWriter, r *http.Request) {
	ac, apiErr := a.gate.Authenticate(r)
	if apiErr != nil {
		a.writeGateError(w, translate.FormatOpenAI, apiErr)
		return
	}
	type modelEntry struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}
	data := []modelEntry{}
	now := time.Now().Unix()

	models, err := a.st.ListEnabledModels()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for _, m := range models {
		// allowlist key menerima public_id maupun alias
		if !ac.ModelAllowed(m.PublicID) && (m.Alias == "" || !ac.ModelAllowed(m.Alias)) {
			continue
		}
		data = append(data, modelEntry{ID: modelDisplayID(m), Object: "model", Created: now, OwnedBy: m.ProviderName})
	}
	if combos, err := a.st.ListCombos(); err == nil {
		for _, c := range combos {
			if ac.ModelAllowed(c.Name) {
				data = append(data, modelEntry{ID: c.Name, Object: "model", Created: now, OwnedBy: "combo"})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// usageMe menangani GET /v1/usage/me: sisa kuota dan pemakaian (§9).
func (a *App) usageMe(w http.ResponseWriter, r *http.Request) {
	ac, apiErr := a.gate.Authenticate(r)
	if apiErr != nil {
		a.writeGateError(w, translate.FormatOpenAI, apiErr)
		return
	}
	type quotaView struct {
		Scope        string  `json:"scope"`
		Period       string  `json:"period"`
		TokenLimit   int64   `json:"token_limit"`
		UsedTokens   int64   `json:"used_tokens"`
		RequestLimit int64   `json:"request_limit"`
		UsedRequests int64   `json:"used_requests"`
		CostLimitUSD float64 `json:"cost_limit_usd"`
		UsedCostUSD  float64 `json:"used_cost_usd"`
		ResetAt      string  `json:"reset_at"`
	}
	quotas := []quotaView{}
	for _, q := range collectUserKeyQuotas(a.st, ac.User.ID, ac.Key.ID) {
		quotas = append(quotas, quotaView{
			Scope: q.Scope, Period: q.Period,
			TokenLimit: q.TokenLimit, UsedTokens: q.UsedTokens,
			RequestLimit: q.RequestLimit, UsedRequests: q.UsedRequests,
			CostLimitUSD: q.CostLimitUSD, UsedCostUSD: q.UsedCost,
			ResetAt: q.ResetAt,
		})
	}

	// pemakaian hari ini dari log (FR-5.1)
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	res, err := usage.GetAnalytics(a.st, from, now.Add(time.Minute))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var mine usage.UserUsage
	for _, u := range res.PerUser {
		if u.UserID == ac.User.ID {
			mine = u
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"key": map[string]any{
			"id":     ac.Key.ID,
			"prefix": ac.Key.Prefix,
			"name":   ac.Key.Name,
		},
		"quotas": quotas,
		"today": map[string]any{
			"requests":   mine.Requests,
			"tokens_in":  mine.TokensIn,
			"tokens_out": mine.TokensOut,
			"cost_usd":   fmt.Sprintf("%.6f", mine.CostUSD),
		},
	})
}

// collectUserKeyQuotas gabungan kuota user + key.
func collectUserKeyQuotas(st *store.Store, userID, keyID int64) []*store.Quota {
	var out []*store.Quota
	if qs, err := st.ListQuotasByScope("user", userID); err == nil {
		out = append(out, qs...)
	}
	if qs, err := st.ListQuotasByScope("key", keyID); err == nil {
		out = append(out, qs...)
	}
	return out
}

var _ = apigate.ErrModelNotFound
