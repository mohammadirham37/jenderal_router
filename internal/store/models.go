package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// Model adalah model terdaftar milik provider (F-02).
type Model struct {
	ID            int64    `json:"id"`
	ProviderID    int64    `json:"provider_id"`
	UpstreamName  string   `json:"upstream_name"`
	PublicID      string   `json:"public_id"` // prefix/nama-model (FR-1.7)
	Alias         string   `json:"alias"`
	DisplayName   string   `json:"display_name"`
	PriceInPer1M  float64  `json:"price_in_per_1m"`
	PriceOutPer1M float64  `json:"price_out_per_1m"`
	ContextWindow int      `json:"context_window"`
	Capabilities  ModelCap `json:"capabilities"`
	Enabled       bool     `json:"enabled"`
	ChatEnabled   bool     `json:"chat_enabled"` // tersedia di chat dashboard
	APIEnabled    bool     `json:"api_enabled"`  // tersedia lewat API /v1
	CreatedAt     string   `json:"created_at"`
	// diisi saat join dengan provider (untuk UI & router)
	ProviderName    string `json:"provider_name,omitempty"`
	ProviderType    string `json:"provider_type,omitempty"`
	ProviderPrefix  string `json:"provider_prefix,omitempty"`
	ProviderEnabled bool   `json:"provider_enabled,omitempty"`
}

// ModelCap kapabilitas model.
type ModelCap struct {
	Tools  bool `json:"tools"`
	Vision bool `json:"vision"`
}

// CreateModel mendaftarkan model; public_id = prefix/upstream.
func (s *Store) CreateModel(providerID int64, upstreamName, alias, displayName string, priceIn, priceOut float64, ctxWindow int, cap ModelCap, enabled bool) (*Model, error) {
	p, err := s.GetProvider(providerID)
	if err != nil {
		return nil, err
	}
	res, err := s.DB.Exec(`INSERT INTO models
		(provider_id, upstream_name, public_id, alias, display_name, price_in_per_1m, price_out_per_1m, context_window, capabilities, enabled)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		providerID, upstreamName, p.Prefix+"/"+upstreamName, alias, displayName, priceIn, priceOut, ctxWindow, toJSON(cap), boolInt(enabled))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetModel(id)
}

// GetModel mengambil model by id.
func (s *Store) GetModel(id int64) (*Model, error) {
	return scanModel(s.DB.QueryRow(modelJoinSelect+` WHERE m.id = ?`, id).Scan)
}

// scanModel helper baris model dengan join provider.
func scanModel(scan func(dest ...any) error) (*Model, error) {
	m := &Model{}
	var enabled, chat, api, caps string
	err := scan(&m.ID, &m.ProviderID, &m.UpstreamName, &m.PublicID, &m.Alias, &m.DisplayName,
		&m.PriceInPer1M, &m.PriceOutPer1M, &m.ContextWindow, &caps, &enabled, &chat, &api, &m.CreatedAt,
		&m.ProviderName, &m.ProviderType, &m.ProviderPrefix, &m.ProviderEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.Enabled = enabled == "1"
	m.ChatEnabled = chat == "1"
	m.APIEnabled = api == "1"
	_ = json.Unmarshal([]byte(caps), &m.Capabilities)
	return m, nil
}

const modelJoinSelect = `SELECT m.id, m.provider_id, m.upstream_name, m.public_id, m.alias, m.display_name,
	m.price_in_per_1m, m.price_out_per_1m, m.context_window, m.capabilities, m.enabled, m.chat_enabled, m.api_enabled, m.created_at,
	p.name, p.type, p.prefix, p.enabled
	FROM models m JOIN providers p ON p.id = m.provider_id`

// ListModels mengembalikan semua model (+info provider).
func (s *Store) ListModels() ([]*Model, error) {
	rows, err := s.DB.Query(modelJoinSelect + ` ORDER BY p.name, m.upstream_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Model
	for rows.Next() {
		m, err := scanModel(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListEnabledModels hanya model milik provider aktif.
func (s *Store) ListEnabledModels() ([]*Model, error) {
	rows, err := s.DB.Query(modelJoinSelect + ` WHERE m.enabled = 1 AND p.enabled = 1 ORDER BY p.name, m.upstream_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Model
	for rows.Next() {
		m, err := scanModel(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// FindModelByPublicOrAlias mencari model berdasarkan public_id atau alias
// (routing FR: resolusi `model` di request).
func (s *Store) FindModelByPublicOrAlias(name string) (*Model, error) {
	m, err := scanModel(s.DB.QueryRow(modelJoinSelect+` WHERE m.public_id = ? OR m.alias = ?`, name, name).Scan)
	return m, err
}

// UpdateModelFields memperbarui alias/harga/capabilities/enabled (FR-1.8).
func (s *Store) UpdateModelFields(id int64, alias, displayName *string, priceIn, priceOut *float64, ctxWindow *int, cap *ModelCap, enabled, chatEnabled, apiEnabled *bool) error {
	m, err := s.GetModel(id)
	if err != nil {
		return err
	}
	if alias != nil {
		m.Alias = *alias
	}
	if displayName != nil {
		m.DisplayName = *displayName
	}
	if priceIn != nil {
		m.PriceInPer1M = *priceIn
	}
	if priceOut != nil {
		m.PriceOutPer1M = *priceOut
	}
	if ctxWindow != nil {
		m.ContextWindow = *ctxWindow
	}
	if cap != nil {
		m.Capabilities = *cap
	}
	if enabled != nil {
		m.Enabled = *enabled
	}
	if chatEnabled != nil {
		m.ChatEnabled = *chatEnabled
	}
	if apiEnabled != nil {
		m.APIEnabled = *apiEnabled
	}
	// mengaktifkan satu kanal ikut menyalakan saklar utama
	if (chatEnabled != nil && *chatEnabled) || (apiEnabled != nil && *apiEnabled) {
		m.Enabled = true
	}
	_, err = s.DB.Exec(`UPDATE models SET alias=?, display_name=?, price_in_per_1m=?, price_out_per_1m=?,
		context_window=?, capabilities=?, enabled=?, chat_enabled=?, api_enabled=? WHERE id=?`,
		m.Alias, m.DisplayName, m.PriceInPer1M, m.PriceOutPer1M, m.ContextWindow, toJSON(m.Capabilities),
		boolInt(m.Enabled), boolInt(m.ChatEnabled), boolInt(m.APIEnabled), id)
	return err
}

// SetModelChannels mengatur kanal chat/api banyak model sekaligus (nil =
// tidak diubah). Menyalakan kanal ikut menyalakan saklar utama enabled.
func (s *Store) SetModelChannels(ids []int64, chatEnabled, apiEnabled *bool) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if chatEnabled != nil {
			if _, err := tx.Exec(`UPDATE models SET chat_enabled = ? WHERE id = ?`, boolInt(*chatEnabled), id); err != nil {
				return err
			}
		}
		if apiEnabled != nil {
			if _, err := tx.Exec(`UPDATE models SET api_enabled = ? WHERE id = ?`, boolInt(*apiEnabled), id); err != nil {
				return err
			}
		}
		if (chatEnabled != nil && *chatEnabled) || (apiEnabled != nil && *apiEnabled) {
			if _, err := tx.Exec(`UPDATE models SET enabled = 1 WHERE id = ?`, id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// UpsertModelSinkron menyisipkan/memperbarui model hasil sinkronisasi (FR-1.6).
func (s *Store) UpsertModelSinkron(providerID int64, upstreamName string, ctxWindow int) error {
	p, err := s.GetProvider(providerID)
	if err != nil {
		return err
	}
	var id int64
	err = s.DB.QueryRow(`SELECT id FROM models WHERE provider_id = ? AND upstream_name = ?`, providerID, upstreamName).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.DB.Exec(`INSERT INTO models (provider_id, upstream_name, public_id, display_name, enabled)
			VALUES (?,?,?,?,1)`, providerID, upstreamName, p.Prefix+"/"+upstreamName, upstreamName)
		return err
	}
	return err
}

// DeleteModel menghapus model.
func (s *Store) DeleteModel(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM models WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Combo (F-06) ----

// Combo adalah rantai model fallback berurutan.
type Combo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	// diisi oleh GetCombo:
	Steps []ComboStep `json:"steps,omitempty"`
}

// ComboStep satu langkah combo.
type ComboStep struct {
	Position  int    `json:"position"`
	ModelID   int64  `json:"model_id"`
	PublicID  string `json:"public_id,omitempty"` // join untuk UI
	TimeoutMs int    `json:"timeout_ms"`
}

// CreateCombo membuat combo + langkah-langkahnya.
func (s *Store) CreateCombo(name, description string, modelIDs []int64) (*Combo, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	res, err := tx.Exec(`INSERT INTO combos (name, description) VALUES (?,?)`, name, description)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	id, _ := res.LastInsertId()
	for i, mid := range modelIDs {
		if _, err := tx.Exec(`INSERT INTO combo_steps (combo_id, position, model_id, timeout_ms) VALUES (?,?,?,?)`,
			id, i+1, mid, 120000); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetCombo(id)
}

// GetCombo mengambil combo beserta langkah berurutan.
func (s *Store) GetCombo(id int64) (*Combo, error) {
	c := &Combo{}
	err := s.DB.QueryRow(`SELECT id, name, description, created_at FROM combos WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.Description, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(`SELECT cs.position, cs.model_id, m.public_id, cs.timeout_ms
		FROM combo_steps cs JOIN models m ON m.id = cs.model_id
		WHERE cs.combo_id = ? ORDER BY cs.position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var st ComboStep
		var pub sql.NullString
		if err := rows.Scan(&st.Position, &st.ModelID, &pub, &st.TimeoutMs); err != nil {
			return nil, err
		}
		st.PublicID = fromNull(pub)
		c.Steps = append(c.Steps, st)
	}
	return c, rows.Err()
}

// GetComboByName mencari combo berdasarkan nama (field `model` di request).
func (s *Store) GetComboByName(name string) (*Combo, error) {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM combos WHERE name = ?`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetCombo(id)
}

// ListCombos semua combo dengan langkah.
func (s *Store) ListCombos() ([]*Combo, error) {
	rows, err := s.DB.Query(`SELECT id FROM combos ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []*Combo
	for _, id := range ids {
		c, err := s.GetCombo(id)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// UpdateCombo mengganti nama/deskripsi/langkah.
func (s *Store) UpdateCombo(id int64, name, description *string, modelIDs []int64) error {
	c, err := s.GetCombo(id)
	if err != nil {
		return err
	}
	if name != nil {
		c.Name = *name
	}
	if description != nil {
		c.Description = *description
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE combos SET name=?, description=? WHERE id=?`, c.Name, c.Description, id); err != nil {
		tx.Rollback()
		return err
	}
	if modelIDs != nil {
		if _, err := tx.Exec(`DELETE FROM combo_steps WHERE combo_id = ?`, id); err != nil {
			tx.Rollback()
			return err
		}
		for i, mid := range modelIDs {
			if _, err := tx.Exec(`INSERT INTO combo_steps (combo_id, position, model_id, timeout_ms) VALUES (?,?,?,?)`,
				id, i+1, mid, 120000); err != nil {
				tx.Rollback()
				return err
			}
		}
	}
	return tx.Commit()
}

// DeleteCombo menghapus combo.
func (s *Store) DeleteCombo(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM combos WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Kuota (F-09) ----

// Quota batas pemakaian per user atau per key.
type Quota struct {
	ID           int64   `json:"id"`
	Scope        string  `json:"scope"` // user|key
	ScopeID      int64   `json:"scope_id"`
	Period       string  `json:"period"` // day|month
	TokenLimit   int64   `json:"token_limit"`
	RequestLimit int64   `json:"request_limit"`
	CostLimitUSD float64 `json:"cost_limit_usd"`
	UsedTokens   int64   `json:"used_tokens"`
	UsedRequests int64   `json:"used_requests"`
	UsedCost     float64 `json:"used_cost"`
	ResetAt      string  `json:"reset_at"`
	Source       string  `json:"source"` // default|custom
}

// PutQuota menyimpan (upsert) kuota khusus dan mengembalikan barisnya.
// Baris default yang sudah ada berubah menjadi khusus (pemakaian tetap).
func (s *Store) PutQuota(scope string, scopeID int64, period string, tokenLimit, requestLimit int64, costLimit float64) (*Quota, error) {
	_, err := s.DB.Exec(`INSERT INTO quotas (scope, scope_id, period, token_limit, request_limit, cost_limit_usd, source)
		VALUES (?,?,?,?,?,?,'custom')
		ON CONFLICT(scope, scope_id, period) DO UPDATE SET
		token_limit = excluded.token_limit, request_limit = excluded.request_limit,
		cost_limit_usd = excluded.cost_limit_usd, source = 'custom'`,
		scope, scopeID, period, tokenLimit, requestLimit, costLimit)
	if err != nil {
		return nil, err
	}
	return s.GetQuota(scope, scopeID, period)
}

// GetQuota mengambil kuota (dengan reset otomatis bila jatuh tempo).
func (s *Store) GetQuota(scope string, scopeID int64, period string) (*Quota, error) {
	q := &Quota{}
	err := s.DB.QueryRow(`SELECT id, scope, scope_id, period, token_limit, request_limit, cost_limit_usd,
		used_tokens, used_requests, used_cost, reset_at, source FROM quotas
		WHERE scope = ? AND scope_id = ? AND period = ?`, scope, scopeID, period).
		Scan(&q.ID, &q.Scope, &q.ScopeID, &q.Period, &q.TokenLimit, &q.RequestLimit, &q.CostLimitUSD,
			&q.UsedTokens, &q.UsedRequests, &q.UsedCost, &q.ResetAt, &q.Source)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return q, nil
}

// ListQuotasByScope semua kuota milik user atau key.
func (s *Store) ListQuotasByScope(scope string, scopeID int64) ([]*Quota, error) {
	rows, err := s.DB.Query(`SELECT id, scope, scope_id, period, token_limit, request_limit, cost_limit_usd,
		used_tokens, used_requests, used_cost, reset_at, source FROM quotas
		WHERE scope = ? AND scope_id = ? ORDER BY period`, scope, scopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Quota
	for rows.Next() {
		q := &Quota{}
		if err := rows.Scan(&q.ID, &q.Scope, &q.ScopeID, &q.Period, &q.TokenLimit, &q.RequestLimit,
			&q.CostLimitUSD, &q.UsedTokens, &q.UsedRequests, &q.UsedCost, &q.ResetAt, &q.Source); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// RecordQuotaUsage menambah pemakaian atomik; mengembalikan baris pasca-update.
func (s *Store) RecordQuotaUsage(scope string, scopeID int64, tokens int64, cost float64) error {
	_, err := s.DB.Exec(`UPDATE quotas SET used_tokens = used_tokens + ?, used_requests = used_requests + 1,
		used_cost = used_cost + ? WHERE scope = ? AND scope_id = ?`, tokens, cost, scope, scopeID)
	return err
}

// ResetQuota mengosongkan pemakaian dan set reset_at berikutnya.
func (s *Store) ResetQuota(id int64, nextReset string) error {
	_, err := s.DB.Exec(`UPDATE quotas SET used_tokens = 0, used_requests = 0, used_cost = 0, reset_at = ? WHERE id = ?`,
		nextReset, id)
	return err
}

// QuotasDueReset kuota yang reset_at-nya lewat (worker reset FR-4.5).
func (s *Store) QuotasDueReset(nowISO string) ([]*Quota, error) {
	rows, err := s.DB.Query(`SELECT id, scope, scope_id, period, token_limit, request_limit, cost_limit_usd,
		used_tokens, used_requests, used_cost, reset_at, source FROM quotas
		WHERE reset_at != '' AND reset_at < ?`, nowISO)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Quota
	for rows.Next() {
		q := &Quota{}
		if err := rows.Scan(&q.ID, &q.Scope, &q.ScopeID, &q.Period, &q.TokenLimit, &q.RequestLimit,
			&q.CostLimitUSD, &q.UsedTokens, &q.UsedRequests, &q.UsedCost, &q.ResetAt, &q.Source); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// ---- Kuota default (berlaku untuk member & viewer) ----

// QuotaLimits batas satu periode; semua 0 = tidak ada kuota default.
type QuotaLimits struct {
	TokenLimit   int64   `json:"token_limit"`
	RequestLimit int64   `json:"request_limit"`
	CostLimitUSD float64 `json:"cost_limit_usd"`
}

// Empty true bila tidak ada batas yang diisi.
func (l QuotaLimits) Empty() bool {
	return l.TokenLimit <= 0 && l.RequestLimit <= 0 && l.CostLimitUSD <= 0
}

// DefaultQuota kuota default per periode (day|month).
type DefaultQuota map[string]QuotaLimits

const defaultQuotaKey = "default_quota"

// QuotaPeriods periode kuota yang dikenal.
var QuotaPeriods = []string{"day", "month"}

// rolesWithDefaultQuota role yang dikenai kuota default (admin tidak,
// agar pengelola tidak terkunci).
const rolesWithDefaultQuota = `('member','viewer')`

// GetDefaultQuota membaca kuota default dari settings.
func (s *Store) GetDefaultQuota() DefaultQuota {
	d := DefaultQuota{}
	if v, _ := s.GetSetting(defaultQuotaKey); v != "" {
		_ = json.Unmarshal([]byte(v), &d)
	}
	return d
}

// SetDefaultQuota menyimpan kuota default lalu menerapkannya ke semua
// member/viewer: baris default diperbarui/ditambah (pemakaian berjalan
// tetap), baris khusus tidak disentuh, dan periode yang dikosongkan
// dihapus. Mengembalikan jumlah user yang kini memakai kuota default.
func (s *Store) SetDefaultQuota(d DefaultQuota) (int64, error) {
	clean := DefaultQuota{}
	for _, p := range QuotaPeriods {
		if l, ok := d[p]; ok && !l.Empty() {
			clean[p] = l
		}
	}
	b, _ := json.Marshal(clean)
	if err := s.SetSetting(defaultQuotaKey, string(b)); err != nil {
		return 0, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, p := range QuotaPeriods {
		l, ok := clean[p]
		if !ok {
			if _, err := tx.Exec(`DELETE FROM quotas WHERE scope = 'user' AND source = 'default' AND period = ?`, p); err != nil {
				return 0, err
			}
			continue
		}
		if _, err := tx.Exec(`INSERT INTO quotas (scope, scope_id, period, token_limit, request_limit, cost_limit_usd, source)
			SELECT 'user', id, ?, ?, ?, ?, 'default' FROM users WHERE role IN `+rolesWithDefaultQuota+`
			ON CONFLICT(scope, scope_id, period) DO UPDATE SET
			token_limit = excluded.token_limit, request_limit = excluded.request_limit,
			cost_limit_usd = excluded.cost_limit_usd
			WHERE quotas.source = 'default'`, p, l.TokenLimit, l.RequestLimit, l.CostLimitUSD); err != nil {
			return 0, err
		}
	}
	// role admin ke atas tidak memakai kuota default
	if _, err := tx.Exec(`DELETE FROM quotas WHERE scope = 'user' AND source = 'default'
		AND scope_id IN (SELECT id FROM users WHERE role NOT IN ` + rolesWithDefaultQuota + `)`); err != nil {
		return 0, err
	}
	var n int64
	if err := tx.QueryRow(`SELECT COUNT(DISTINCT scope_id) FROM quotas WHERE scope = 'user' AND source = 'default'`).Scan(&n); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// SyncDefaultQuotaForUser menyelaraskan kuota default satu user dengan
// role-nya (dipanggil saat user dibuat atau role berubah).
func (s *Store) SyncDefaultQuotaForUser(userID int64, role string) error {
	if role != RoleMember && role != RoleViewer {
		_, err := s.DB.Exec(`DELETE FROM quotas WHERE scope = 'user' AND scope_id = ? AND source = 'default'`, userID)
		return err
	}
	for p, l := range s.GetDefaultQuota() {
		if _, err := s.DB.Exec(`INSERT INTO quotas (scope, scope_id, period, token_limit, request_limit, cost_limit_usd, source)
			VALUES ('user', ?, ?, ?, ?, ?, 'default') ON CONFLICT(scope, scope_id, period) DO NOTHING`,
			userID, p, l.TokenLimit, l.RequestLimit, l.CostLimitUSD); err != nil {
			return err
		}
	}
	return nil
}

// UseDefaultQuota menghapus kuota khusus user untuk satu periode lalu
// memasang kuota default (bila ada) — tombol "kembali ke default". Baris
// yang sudah default tidak disentuh agar pemakaian berjalan tetap.
func (s *Store) UseDefaultQuota(userID int64, role, period string) error {
	if _, err := s.DB.Exec(`DELETE FROM quotas WHERE scope = 'user' AND scope_id = ? AND period = ? AND source = 'custom'`, userID, period); err != nil {
		return err
	}
	return s.SyncDefaultQuotaForUser(userID, role)
}

// SetQuotaResetAt mengisi jadwal reset tanpa mengubah pemakaian.
func (s *Store) SetQuotaResetAt(id int64, resetAt string) error {
	_, err := s.DB.Exec(`UPDATE quotas SET reset_at = ? WHERE id = ?`, resetAt, id)
	return err
}

// QuotasWithoutReset kuota yang belum punya jadwal reset.
func (s *Store) QuotasWithoutReset() ([]*Quota, error) {
	rows, err := s.DB.Query(`SELECT id, period FROM quotas WHERE reset_at = ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Quota
	for rows.Next() {
		q := &Quota{}
		if err := rows.Scan(&q.ID, &q.Period); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}
