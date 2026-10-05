package store

import "testing"

func TestAutoTitleFromFirstUserMessage(t *testing.T) {
	s, err := NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	u, err := s.CreateUser("titler@test", "Sandi-Kuat-123", RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(u.ID, "Chat baru", "m1", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.AddMessage(c.ID, "assistant", "jawaban dulu", "m1", "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMessage(c.ID, "user", "apa itu llama.cpp? jelaskan singkat", "", ""); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetConversation(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "apa itu llama.cpp? jelaskan singkat" {
		t.Fatalf("judul=%q", got.Title)
	}

	// pesan user berikutnya tidak menimpa judul
	if _, err := s.AddMessage(c.ID, "user", "pertanyaan kedua", "", ""); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetConversation(c.ID)
	if got2.Title != "apa itu llama.cpp? jelaskan singkat" {
		t.Fatalf("judul berubah: %q", got2.Title)
	}
}

func TestRepairConversationTitles(t *testing.T) {
	s, err := NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	u, err := s.CreateUser("repair@test", "Sandi-Kuat-123", RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(u.ID, "Chat baru", "m1", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMessage(c.ID, "user", "pertanyaan pertama lama", "", ""); err != nil {
		t.Fatal(err)
	}
	// simulasi percakapan lama: judul dikembalikan ke default lalu repair
	if _, err := s.DB.Exec(`UPDATE conversations SET title = 'Chat baru' WHERE id = ?`, c.ID); err != nil {
		t.Fatal(err)
	}
	s.RepairConversationTitles()

	got, err := s.GetConversation(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "pertanyaan pertama lama" {
		t.Fatalf("repair gagal: %q", got.Title)
	}
}
