package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/translate"
)

// Pencatatan isi request API (opt-in). Aktif bila salah satu saklar menyala:
// env JR_LOG_PROMPTS, setting global "log_content", saklar per user, atau
// saklar per API key. Isi disimpan di request_logs.content_debug (JSON),
// rahasia yang umum (API key, token, kata sandi, nomor kartu) disamarkan,
// ukurannya dibatasi, dan dikosongkan otomatis setelah contentRetention.

const (
	contentRetention   = 7 * 24 * time.Hour
	maxLoggedRequest   = 48 << 10 // total isi pesan request
	maxLoggedMessage   = 8 << 10  // per pesan
	maxLoggedResponse  = 16 << 10
	settingLogContent  = "log_content"
	truncatedMarkerFmt = "\n…[%d karakter dipotong]…\n"
)

// contentLogging apakah isi request dari user/key ini perlu dicatat.
func (a *App) contentLogging(userID int64, key *store.APIKey) bool {
	if a.cfg.LogPrompts || (key != nil && key.LogContent) {
		return true
	}
	if v, _ := a.st.GetSetting(settingLogContent); v == "true" {
		return true
	}
	on, _ := a.st.UserLogContent(userID)
	return on
}

type loggedToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

type loggedMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []loggedToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// contentLog isi request + jawaban yang dicatat (bentuk JSON tersimpan).
type contentLog struct {
	Request struct {
		Model    string          `json:"model"`
		Stream   bool            `json:"stream"`
		Tools    []string        `json:"tools,omitempty"`
		Messages []loggedMessage `json:"messages"`
	} `json:"request"`
	Response struct {
		Content   string           `json:"content,omitempty"`
		ToolCalls []loggedToolCall `json:"tool_calls,omitempty"`
	} `json:"response"`
	Error     string `json:"error,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// clip memotong teks panjang di tengah (awal & akhir paling informatif).
func (c *contentLog) clip(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	c.Truncated = true
	head := limit / 3
	tail := limit - head
	return string(r[:head]) + fmt.Sprintf(truncatedMarkerFmt, len(r)-limit) + string(r[len(r)-tail:])
}

// newContentLog merangkum request (gambar tidak disimpan, hanya penanda).
func newContentLog(req *translate.ChatRequest) *contentLog {
	c := &contentLog{}
	c.Request.Model, c.Request.Stream = req.Model, req.Stream
	for _, t := range req.Tools {
		c.Request.Tools = append(c.Request.Tools, t.Name)
	}
	msgs := make([]loggedMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		var b strings.Builder
		for _, p := range m.Content {
			if p.Type == translate.PartImage {
				b.WriteString("[gambar]")
				continue
			}
			b.WriteString(p.Text)
		}
		lm := loggedMessage{Role: m.Role, Content: c.clip(maskSecrets(b.String()), maxLoggedMessage), ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			lm.ToolCalls = append(lm.ToolCalls, loggedToolCall{Name: tc.Name, Arguments: c.clip(maskSecrets(tc.Arguments), maxLoggedMessage)})
		}
		msgs = append(msgs, lm)
	}
	c.Request.Messages = c.fitMessages(msgs)
	return c
}

// fitMessages membatasi total ukuran: pesan system pertama dan pesan
// terbaru dipertahankan, pesan di tengah diganti penanda.
func (c *contentLog) fitMessages(msgs []loggedMessage) []loggedMessage {
	size := func(m loggedMessage) int {
		n := len(m.Content)
		for _, tc := range m.ToolCalls {
			n += len(tc.Arguments)
		}
		return n
	}
	total := 0
	for _, m := range msgs {
		total += size(m)
	}
	if total <= maxLoggedRequest {
		return msgs
	}
	c.Truncated = true
	var head []loggedMessage
	rest := msgs
	budget := maxLoggedRequest
	if len(rest) > 0 && rest[0].Role == translate.RoleSystem {
		head, rest = rest[:1], rest[1:]
		budget -= size(head[0])
	}
	keep := 0
	for i := len(rest) - 1; i >= 0; i-- {
		if budget-size(rest[i]) < 0 && keep > 0 {
			break
		}
		budget -= size(rest[i])
		keep++
	}
	out := append([]loggedMessage(nil), head...)
	if dropped := len(rest) - keep; dropped > 0 {
		out = append(out, loggedMessage{Role: "info", Content: fmt.Sprintf("[%d pesan sebelumnya tidak disimpan karena batas ukuran log]", dropped)})
	}
	return append(out, rest[len(rest)-keep:]...)
}

// setResponse mengisi jawaban model.
func (c *contentLog) setResponse(content string, calls []translate.ToolCall) {
	if c == nil {
		return
	}
	c.Response.Content = c.clip(maskSecrets(content), maxLoggedResponse)
	for _, tc := range calls {
		c.Response.ToolCalls = append(c.Response.ToolCalls, loggedToolCall{Name: tc.Name, Arguments: c.clip(maskSecrets(tc.Arguments), maxLoggedMessage)})
	}
}

func (c *contentLog) setError(err string) {
	if c != nil && err != "" {
		c.Error = maskSecrets(err)
	}
}

// render JSON untuk kolom content_debug ("" bila pencatatan mati).
func (c *contentLog) render() string {
	if c == nil {
		return ""
	}
	b, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(b)
}

// toolCallAccumulator merakit potongan tool call dari stream.
type toolCallAccumulator struct {
	order []int
	calls map[int]*translate.ToolCall
}

func (t *toolCallAccumulator) add(deltas []translate.ToolCallDelta) {
	if t.calls == nil {
		t.calls = map[int]*translate.ToolCall{}
	}
	for _, d := range deltas {
		c := t.calls[d.Index]
		if c == nil {
			c = &translate.ToolCall{}
			t.calls[d.Index] = c
			t.order = append(t.order, d.Index)
		}
		c.Name += d.Name
		c.Arguments += d.Arguments
	}
}

func (t *toolCallAccumulator) list() []translate.ToolCall {
	out := make([]translate.ToolCall, 0, len(t.order))
	for _, i := range t.order {
		out = append(out, *t.calls[i])
	}
	return out
}

// ---- penyamaran rahasia ----

var (
	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\b(?:sk|pk|rk|jr)-[A-Za-z0-9_\-]{16,}`),                           // OpenAI/Anthropic/JenderalRouter
		regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{30,}`),                                       // Google API key
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),                                            // AWS access key
		regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`),                                    // GitHub token
		regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9\-]{10,}`),                                 // Slack token
		regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}`), // JWT
	}
	bearerRe   = regexp.MustCompile(`(?i)(\bbearer\s+)[A-Za-z0-9._~+/\-]{16,}=*`)
	assignRe   = regexp.MustCompile(`(?i)\b((?:password|passwd|pwd|kata[ _]?sandi|secret|api[_\-]?key|access[_\-]?token|token)["']?\s*[:=]\s*["']?)([^\s"',;]{4,})`)
	cardNumRe  = regexp.MustCompile(`\b\d(?:[ \-]?\d){12,18}\b`)
	secretMask = "[disamarkan]"
)

// maskSecrets menyamarkan pola rahasia umum dalam teks yang dicatat (NFR-14).
func maskSecrets(s string) string {
	if s == "" {
		return s
	}
	for _, re := range secretPatterns {
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			return m[:min(4, len(m))] + "…" + secretMask
		})
	}
	s = bearerRe.ReplaceAllString(s, "${1}"+secretMask)
	s = assignRe.ReplaceAllString(s, "${1}"+secretMask)
	s = cardNumRe.ReplaceAllStringFunc(s, func(m string) string {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, m)
		if len(digits) < 13 || !luhnValid(digits) {
			return m
		}
		return "[kartu ****" + digits[len(digits)-4:] + "]"
	})
	return s
}

func luhnValid(digits string) bool {
	sum, double := 0, false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// purgeLogContent mengosongkan isi request yang lebih tua dari batas retensi.
func (a *App) purgeLogContent(now time.Time) (int64, error) {
	res, err := a.st.DB.Exec(`UPDATE request_logs SET content_debug = '' WHERE content_debug != '' AND ts < ?`,
		now.Add(-contentRetention).UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---- HTTP ----

// contentLogGlobal saklar global (env JR_LOG_PROMPTS atau setting admin).
func (a *App) contentLogGlobal() bool {
	if a.cfg.LogPrompts {
		return true
	}
	v, _ := a.st.GetSetting(settingLogContent)
	return v == "true"
}

// handleSetUserLogContent PUT /api/admin/users/{id}/log-content — {enabled}.
func (a *App) handleSetUserLogContent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, ok := a.manageable(w, r, id)
	if !ok {
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	if err := a.st.SetUserLogContent(id, req.Enabled); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "user.log_content", "users/"+fmtInt(id), nil, map[string]any{"enabled": req.Enabled, "email": u.Email})
	writeJSON(w, 200, map[string]any{"ok": true, "enabled": req.Enabled})
}

// handleSetKeyLogContent PUT /api/admin/keys/{id}/log-content — {enabled}.
func (a *App) handleSetKeyLogContent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	k, ok := a.manageableKey(w, r, id)
	if !ok {
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	if err := a.st.SetKeyLogContent(id, req.Enabled); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.gate.InvalidateKey(k.KeyHash) // key di-cache gerbang API → muat ulang saklar
	a.audit(r, "key.log_content", "keys/"+fmtInt(id), nil, map[string]any{"enabled": req.Enabled, "key": k.Name})
	writeJSON(w, 200, map[string]any{"ok": true, "enabled": req.Enabled})
}

// handleLogContent GET /api/admin/logs/{id}/content — isi request & jawaban
// satu baris log. Hanya untuk user yang boleh dikelola aktor; setiap akses
// tercatat di audit log.
func (a *App) handleLogContent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var ts, content string
	var userID, keyID sql.NullInt64
	err := a.st.DB.QueryRow(`SELECT ts, user_id, key_id, content_debug FROM request_logs WHERE id = ?`, id).
		Scan(&ts, &userID, &keyID, &content)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 404, map[string]string{"error": "log tidak ditemukan"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if !userID.Valid {
		writeJSON(w, 404, map[string]string{"error": "log tanpa pemilik"})
		return
	}
	owner, ok := a.manageable(w, r, userID.Int64)
	if !ok {
		return
	}
	if content == "" {
		writeJSON(w, 404, map[string]string{"error": "isi request tidak tersedia: tidak dicatat, atau sudah lewat 7 hari"})
		return
	}
	resp := map[string]any{"id": id, "ts": ts, "user_email": owner.Email, "content": json.RawMessage(content)}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		resp["expires_at"] = t.Add(contentRetention).UTC().Format(time.RFC3339)
	}
	if keyID.Valid {
		if k, err := a.st.GetAPIKey(keyID.Int64); err == nil {
			resp["key_name"], resp["key_prefix"] = k.Name, k.Prefix
		}
	}
	a.audit(r, "log.content.view", "request_logs/"+fmtInt(id), nil, map[string]string{"owner": owner.Email})
	writeJSON(w, 200, resp)
}
