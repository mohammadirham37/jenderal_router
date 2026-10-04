package api

import (
	"encoding/json"
	"net/http"
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
