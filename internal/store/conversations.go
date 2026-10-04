package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// Conversation percakapan playground milik user (FR-7.2/7.3).
type Conversation struct {
	ID           int64    `json:"id"`
	UserID       int64    `json:"user_id"`
	Title        string   `json:"title"`
	Model        string   `json:"model"`
	SystemPrompt string   `json:"system_prompt"`
	Temperature  *float64 `json:"temperature"`
	MaxTokens    *int     `json:"max_tokens"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

// Message satu pesan dalam percakapan.
type Message struct {
	ID             int64  `json:"id"`
	ConversationID int64  `json:"conversation_id"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	Model          string `json:"model"`
	ProviderName   string `json:"provider_name"`
	CreatedAt      string `json:"created_at"`
}

// CreateConversation membuat percakapan baru.
func (s *Store) CreateConversation(userID int64, title, model, systemPrompt string, temp *float64, maxTokens *int) (*Conversation, error) {
	if title == "" {
		title = "Percakapan baru"
	}
	res, err := s.DB.Exec(`INSERT INTO conversations (user_id, title, model) VALUES (?,?,?)`, userID, title, model)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	// settings tersimpan via kolom title/model + settings JSON kecil
	if systemPrompt != "" || temp != nil || maxTokens != nil {
		sett := toJSON(map[string]any{
			"system_prompt": systemPrompt, "temperature": temp, "max_tokens": maxTokens,
		})
		_, _ = s.DB.Exec(`INSERT INTO settings (key, value) VALUES (?,?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, convSettingsKey(id), sett)
	}
	return s.GetConversation(id)
}

func convSettingsKey(id int64) string {
	return "conv:" + int64ToStr(id)
}

func int64ToStr(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [21]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// GetConversation mengambil percakapan (dengan settings).
func (s *Store) GetConversation(id int64) (*Conversation, error) {
	c := &Conversation{}
	err := s.DB.QueryRow(`SELECT id, user_id, title, model, created_at, updated_at FROM conversations WHERE id = ?`, id).
		Scan(&c.ID, &c.UserID, &c.Title, &c.Model, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var sett string
	_ = s.DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, convSettingsKey(id)).Scan(&sett)
	if sett != "" {
		var m map[string]any
		if err := jsonUnmarshalStore([]byte(sett), &m); err == nil {
			if sp, ok := m["system_prompt"].(string); ok {
				c.SystemPrompt = sp
			}
			if t, ok := m["temperature"].(float64); ok {
				c.Temperature = &t
			}
			if mt, ok := m["max_tokens"].(float64); ok {
				v := int(mt)
				c.MaxTokens = &v
			}
		}
	}
	return c, nil
}

// ListConversations milik user, terbaru dulu.
func (s *Store) ListConversations(userID int64) ([]*Conversation, error) {
	rows, err := s.DB.Query(`SELECT id, user_id, title, model, created_at, updated_at
		FROM conversations WHERE user_id = ? ORDER BY updated_at DESC, id DESC LIMIT 200`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Conversation
	for rows.Next() {
		c := &Conversation{}
		if err := rows.Scan(&c.ID, &c.UserID, &c.Title, &c.Model, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateConversation mengubah judul/model/settings.
func (s *Store) UpdateConversation(id int64, title, model, systemPrompt *string, temp *float64, maxTokens *int) error {
	c, err := s.GetConversation(id)
	if err != nil {
		return err
	}
	if title != nil {
		c.Title = *title
	}
	if model != nil {
		c.Model = *model
	}
	_, err = s.DB.Exec(`UPDATE conversations SET title=?, model=?, updated_at=datetime('now') WHERE id=?`,
		c.Title, c.Model, id)
	if err != nil {
		return err
	}
	// simpan settings bila ada yang diubah
	if systemPrompt != nil || temp != nil || maxTokens != nil {
		sett := toJSON(map[string]any{
			"system_prompt": derefStr(systemPrompt, c.SystemPrompt),
			"temperature":   tempOr(temp, c.Temperature),
			"max_tokens":    maxTokens,
		})
		_, err = s.DB.Exec(`INSERT INTO settings (key, value) VALUES (?,?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, convSettingsKey(id), sett)
	}
	return err
}

// TouchConversation memperbarui updated_at.
func (s *Store) TouchConversation(id int64) {
	_, _ = s.DB.Exec(`UPDATE conversations SET updated_at = datetime('now') WHERE id = ?`, id)
}

// DeleteConversation menghapus percakapan + pesannya.
func (s *Store) DeleteConversation(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM conversations WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, _ = s.DB.Exec(`DELETE FROM settings WHERE key = ?`, convSettingsKey(id))
	return nil
}

// AddMessage menambah pesan ke percakapan.
func (s *Store) AddMessage(conversationID int64, role, content, model, providerName string) (*Message, error) {
	res, err := s.DB.Exec(`INSERT INTO messages (conversation_id, role, content, model, provider_name) VALUES (?,?,?,?,?)`,
		conversationID, role, content, model, providerName)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	m := &Message{ID: id, ConversationID: conversationID, Role: role, Content: content,
		Model: model, ProviderName: providerName}
	return m, nil
}

// ListMessages pesan percakapan urut waktu.
func (s *Store) ListMessages(conversationID int64) ([]*Message, error) {
	rows, err := s.DB.Query(`SELECT id, conversation_id, role, content, model, provider_name, created_at
		FROM messages WHERE conversation_id = ? ORDER BY id`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		m := &Message{}
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Model, &m.ProviderName, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetProviderByPrefix mencari provider dari prefix (mis. "local").
func (s *Store) GetProviderByPrefix(prefix string) (*Provider, error) {
	p := &Provider{}
	var enabled int
	var settings string
	err := s.DB.QueryRow(`SELECT id, type, name, prefix, base_url, settings, enabled, created_at
		FROM providers WHERE prefix = ?`, prefix).
		Scan(&p.ID, &p.Type, &p.Name, &p.Prefix, &p.BaseURL, &settings, &enabled, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Enabled = enabled == 1
	p.Settings = jsonRaw(settings)
	return p, nil
}

func derefStr(p *string, def string) string {
	if p != nil {
		return *p
	}
	return def
}

func tempOr(nv *float64, old *float64) *float64 {
	if nv != nil {
		return nv
	}
	return old
}

// helper kecil untuk conversations.go
func jsonUnmarshalStore(data []byte, v any) error { return json.Unmarshal(data, v) }

func jsonRaw(s string) json.RawMessage { return json.RawMessage(s) }
