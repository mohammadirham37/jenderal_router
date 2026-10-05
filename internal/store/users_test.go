package store

import (
	"bytes"
	"testing"
	"time"

	"github.com/jenderal/jenderalrouter/internal/crypto"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	SetMasterKey(bytes.Repeat([]byte{42}, 32)) // kunci test untuk enkripsi secret
	s, err := NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUserCRUD(t *testing.T) {
	s := newTestStore(t)

	u, err := s.CreateUser("admin@local", "Sandi-Kuat-123", RoleSuperAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 || u.Role != RoleSuperAdmin || u.Status != "active" {
		t.Fatalf("user tidak sah: %+v", u)
	}

	got, err := s.GetUserByEmail("admin@local")
	if err != nil || got.ID != u.ID {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if !crypto.VerifyPassword(got.PasswordHash, "Sandi-Kuat-123") {
		t.Error("password hash tidak cocok")
	}

	newRole := RoleAdmin
	if err := s.UpdateUserFields(u.ID, nil, &newRole, nil, nil, nil); err != nil {
		t.Fatalf("UpdateUserFields: %v", err)
	}
	got, _ = s.GetUser(u.ID)
	if got.Role != RoleAdmin {
		t.Errorf("role = %s", got.Role)
	}

	users, _ := s.ListUsers()
	if len(users) != 1 {
		t.Errorf("ListUsers = %d", len(users))
	}

	if err := s.DeleteUser(u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := s.GetUser(u.ID); err != ErrNotFound {
		t.Errorf("harus ErrNotFound, dapat %v", err)
	}
}

func TestDuplicateEmailRejected(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateUser("a@b.c", "Sandi-Kuat-123", RoleMember); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("a@b.c", "Sandi-Kuat-123", RoleMember); err == nil {
		t.Fatal("email duplikat harus ditolak")
	}
}

func TestCountSuperAdmins(t *testing.T) {
	s := newTestStore(t)
	n, _ := s.CountSuperAdmins()
	if n != 0 {
		t.Fatalf("awal harus 0, dapat %d", n)
	}
	s.CreateUser("root@local", "Sandi-Kuat-123", RoleSuperAdmin)
	n, _ = s.CountSuperAdmins()
	if n != 1 {
		t.Errorf("harus 1, dapat %d", n)
	}
}

func TestAPIKeyLifecycle(t *testing.T) {
	s := newTestStore(t)
	u, _ := s.CreateUser("dev@local", "Sandi-Kuat-123", RoleMember)

	plain, hash, err := crypto.NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	k, err := s.CreateAPIKey(u.ID, "utama", plain, hash, "", "oa/gpt-5.4, coding-hemat", "10.0.0.0/8", 60, 100000, "")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if k.Prefix != plain[:8] {
		t.Errorf("prefix = %s", k.Prefix)
	}
	if len(k.AllowedList) != 2 || k.AllowedList[0] != "oa/gpt-5.4" {
		t.Errorf("AllowedList = %v", k.AllowedList)
	}

	byHash, err := s.GetAPIKeyByHash(hash)
	if err != nil || byHash.ID != k.ID {
		t.Fatalf("GetAPIKeyByHash: %v", err)
	}

	if !k.APIKeyValid(time.Now()) {
		t.Error("key baru harus valid")
	}
	if !k.IPAllowed("10.1.2.3") || k.IPAllowed("192.168.99.99") {
		t.Error("IP allowlist 10/8 salah")
	}

	if err := s.RevokeAPIKey(k.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	byHash, _ = s.GetAPIKeyByHash(hash)
	if byHash.APIKeyValid(time.Now()) {
		t.Error("key tercabut harus tidak valid")
	}
	if err := s.DeleteAPIKey(k.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestAPIKeyExpiry(t *testing.T) {
	s := newTestStore(t)
	u, _ := s.CreateUser("x@y.z", "Sandi-Kuat-123", RoleMember)
	plain, hash, _ := crypto.NewAPIKey()
	exp := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	k, err := s.CreateAPIKey(u.ID, "lama", plain, hash, "", "*", "", 0, 0, exp)
	if err != nil {
		t.Fatal(err)
	}
	if k.APIKeyValid(time.Now()) {
		t.Error("key kedaluwarsa harus tidak valid")
	}
}

func TestSessionsAndAudit(t *testing.T) {
	s := newTestStore(t)
	u, _ := s.CreateUser("adm@local", "Sandi-Kuat-123", RoleSuperAdmin)

	token, csrf, err := s.CreateSession(u.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.GetSession(token)
	if err != nil || sess.UserID != u.ID {
		t.Fatalf("GetSession: %v", err)
	}
	if csrf == "" {
		t.Error("csrf kosong")
	}

	if err := s.Audit(u.ID, "user.create", "users/1", nil, map[string]string{"email": "adm@local"}); err != nil {
		t.Fatalf("Audit: %v", err)
	}
	entries, err := s.ListAudit(10)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(entries) != 1 || entries[0].Action != "user.create" {
		t.Fatalf("audit entries = %+v", entries)
	}

	if _, err := s.PurgeExpiredSessions(); err != nil {
		t.Fatal(err)
	}
	// sesi masih aktif (ttl 1 menit, purge hanya hapus kedaluwarsa)
	if _, err := s.GetSession(token); err != nil {
		t.Fatalf("sesi harus masih ada: %v", err)
	}

	if err := s.SetSetting("wizard_done", "1"); err != nil {
		t.Fatal(err)
	}
	v, _ := s.GetSetting("wizard_done")
	if v != "1" {
		t.Errorf("setting = %q", v)
	}

	if err := s.DeleteSession(token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(token); err != ErrNotFound {
		t.Errorf("sesi harus terhapus: %v", err)
	}
}

func TestExpiredSessionPurged(t *testing.T) {
	s := newTestStore(t)
	u, _ := s.CreateUser("adm2@local", "Sandi-Kuat-123", RoleAdmin)
	token, _, err := s.CreateSession(u.ID, -time.Minute) // sudah kedaluwarsa
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(token); err != ErrNotFound {
		t.Errorf("sesi kedaluwarsa harus ErrNotFound: %v", err)
	}
}

func TestRoleAtLeast(t *testing.T) {
	if !RoleAtLeast(RoleSuperAdmin, RoleAdmin) {
		t.Error("super_admin >= admin")
	}
	if RoleAtLeast(RoleViewer, RoleMember) {
		t.Error("viewer < member")
	}
	if !RoleAtLeast(RoleMember, RoleMember) {
		t.Error("sama harus lolos")
	}
}
