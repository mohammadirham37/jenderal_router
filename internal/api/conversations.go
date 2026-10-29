package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
)

// Handler riwayat percakapan playground (FR-7.2/7.3). Konten tersimpan per
// user — ini bagian chat playground, bukan log gateway (yang default tanpa
// isi prompt, NFR-14).

func (a *App) handleListConversations(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	convs, err := a.st.ListConversations(ai.user.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if convs == nil {
		convs = []*store.Conversation{}
	}
	writeJSON(w, 200, map[string]any{"conversations": convs})
}

func (a *App) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	var req struct {
		Title        string   `json:"title"`
		Model        string   `json:"model"`
		SystemPrompt string   `json:"system_prompt"`
		Temperature  *float64 `json:"temperature"`
		MaxTokens    *int     `json:"max_tokens"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		req.Title = ""
	}
	c, err := a.st.CreateConversation(ai.user.ID, req.Title, req.Model, req.SystemPrompt, req.Temperature, req.MaxTokens)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"conversation": c})
}

func (a *App) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, err := a.st.GetConversation(id)
	if err != nil || c.UserID != ai.user.ID {
		writeJSON(w, 404, map[string]string{"error": "percakapan tidak ditemukan"})
		return
	}
	msgs, err := a.st.ListMessages(id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"conversation": c, "messages": msgs})
}

func (a *App) handleUpdateConversation(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, err := a.st.GetConversation(id)
	if err != nil || c.UserID != ai.user.ID {
		writeJSON(w, 404, map[string]string{"error": "percakapan tidak ditemukan"})
		return
	}
	var req struct {
		Title        *string  `json:"title"`
		Model        *string  `json:"model"`
		SystemPrompt *string  `json:"system_prompt"`
		Temperature  *float64 `json:"temperature"`
		MaxTokens    *int     `json:"max_tokens"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	if err := a.st.UpdateConversation(id, req.Title, req.Model, req.SystemPrompt, req.Temperature, req.MaxTokens); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	c, _ = a.st.GetConversation(id)
	writeJSON(w, 200, map[string]any{"conversation": c})
}

func (a *App) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, err := a.st.GetConversation(id)
	if err != nil || c.UserID != ai.user.ID {
		writeJSON(w, 404, map[string]string{"error": "percakapan tidak ditemukan"})
		return
	}
	if err := a.st.DeleteConversation(id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

// handleExportConversation mengekspor riwayat sebagai JSON (FR-7.2).
func (a *App) handleExportConversation(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, err := a.st.GetConversation(id)
	if err != nil || c.UserID != ai.user.ID {
		writeJSON(w, 404, map[string]string{"error": "percakapan tidak ditemukan"})
		return
	}
	msgs, _ := a.st.ListMessages(id)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=conversation-"+fmtInt(id)+".json")
	_ = json.NewEncoder(w).Encode(map[string]any{"conversation": c, "messages": msgs})
}

// SaveConversationMessage menyimpan pesan chat (dipanggil handler chat).
func (a *App) SaveConversationMessage(conversationID int64, role, content, model, provider string) error {
	_, err := a.st.AddMessage(conversationID, role, content, model, provider)
	return err
}

var _ = time.Now

// ---- riwayat chat user untuk admin (halaman detail user) ----
// Isi chat bersifat pribadi: hanya user yang boleh dikelola aktor
// (canManageUser) dan setiap akses dicatat di audit log.

// adminChatTarget memuat user target dan memeriksa hak aktor.
func (a *App) adminChatTarget(w http.ResponseWriter, r *http.Request) (*store.User, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return nil, false
	}
	return a.manageable(w, r, id)
}

func (a *App) handleAdminUserConversations(w http.ResponseWriter, r *http.Request) {
	target, ok := a.adminChatTarget(w, r)
	if !ok {
		return
	}
	convs, err := a.st.ListConversations(target.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	counts, err := a.st.CountMessagesByUser(target.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out := []map[string]any{}
	for _, c := range convs {
		out = append(out, map[string]any{
			"id": c.ID, "title": c.Title, "model": c.Model,
			"created_at": c.CreatedAt, "updated_at": c.UpdatedAt, "message_count": counts[c.ID],
		})
	}
	a.audit(r, "user.conversations.list", "users/"+fmtInt(target.ID), nil, nil)
	writeJSON(w, 200, map[string]any{"conversations": out})
}

func (a *App) handleAdminUserConversation(w http.ResponseWriter, r *http.Request) {
	target, ok := a.adminChatTarget(w, r)
	if !ok {
		return
	}
	cid, err := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	if err != nil || cid <= 0 {
		writeJSON(w, 400, map[string]string{"error": "id percakapan tidak sah"})
		return
	}
	c, err := a.st.GetConversation(cid)
	if err != nil || c.UserID != target.ID {
		writeJSON(w, 404, map[string]string{"error": "percakapan tidak ditemukan"})
		return
	}
	msgs, err := a.st.ListMessages(cid)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if msgs == nil {
		msgs = []*store.Message{}
	}
	a.audit(r, "user.conversation.view", "conversations/"+fmtInt(cid), nil,
		map[string]string{"owner": target.Email})
	writeJSON(w, 200, map[string]any{"conversation": c, "messages": msgs})
}
