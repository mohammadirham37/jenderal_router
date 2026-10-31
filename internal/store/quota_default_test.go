package store

import "testing"

func TestDefaultQuotaAppliesToOldAndNewUsers(t *testing.T) {
	s, err := NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	old, _ := s.CreateUser("lama@test", "Sandi-Kuat-123", RoleMember)
	custom, _ := s.CreateUser("khusus@test", "Sandi-Kuat-123", RoleMember)
	admin, _ := s.CreateUser("admin@test", "Sandi-Kuat-123", RoleAdmin)
	if _, err := s.PutQuota("user", custom.ID, "day", 999, 0, 0); err != nil {
		t.Fatal(err)
	}
	s.RecordQuotaUsage("user", custom.ID, 10, 0)

	n, err := s.SetDefaultQuota(DefaultQuota{"day": {TokenLimit: 1000, RequestLimit: 50}, "month": {}})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 { // hanya lama@test; khusus@test sudah punya kuota harian khusus
		t.Fatalf("users_default = %d, want 1", n)
	}
	q, err := s.GetQuota("user", old.ID, "day")
	if err != nil || q.Source != "default" || q.TokenLimit != 1000 || q.RequestLimit != 50 {
		t.Fatalf("old user quota = %+v, %v", q, err)
	}
	if _, err := s.GetQuota("user", old.ID, "month"); err != ErrNotFound {
		t.Errorf("empty month default must not create rows")
	}
	if q, _ := s.GetQuota("user", custom.ID, "day"); q.Source != "custom" || q.TokenLimit != 999 || q.UsedTokens != 10 {
		t.Errorf("custom quota overwritten: %+v", q)
	}
	if _, err := s.GetQuota("user", admin.ID, "day"); err != ErrNotFound {
		t.Errorf("admin must not get default quota")
	}

	// user baru langsung kena default
	baru, _ := s.CreateUser("baru@test", "Sandi-Kuat-123", RoleViewer)
	if q, err := s.GetQuota("user", baru.ID, "day"); err != nil || q.Source != "default" || q.TokenLimit != 1000 {
		t.Fatalf("new user quota = %+v, %v", q, err)
	}

	// ubah default: pemakaian berjalan dipertahankan
	s.RecordQuotaUsage("user", old.ID, 300, 0)
	if _, err := s.SetDefaultQuota(DefaultQuota{"day": {TokenLimit: 5000}}); err != nil {
		t.Fatal(err)
	}
	if q, _ := s.GetQuota("user", old.ID, "day"); q.TokenLimit != 5000 || q.RequestLimit != 0 || q.UsedTokens != 300 {
		t.Errorf("updated default = %+v", q)
	}

	// "ikuti default" pada periode yang sudah default tidak mereset pemakaian
	if err := s.UseDefaultQuota(old.ID, RoleMember, "day"); err != nil {
		t.Fatal(err)
	}
	if q, _ := s.GetQuota("user", old.ID, "day"); q.UsedTokens != 300 {
		t.Errorf("use default on default row reset usage: %+v", q)
	}

	// kembali ke default untuk user khusus
	if err := s.UseDefaultQuota(custom.ID, RoleMember, "day"); err != nil {
		t.Fatal(err)
	}
	if q, _ := s.GetQuota("user", custom.ID, "day"); q.Source != "default" || q.TokenLimit != 5000 {
		t.Errorf("use default = %+v", q)
	}

	// naik jadi admin → kuota default dilepas; turun lagi → dipasang lagi
	role := RoleAdmin
	s.UpdateUserFields(old.ID, nil, &role, nil, nil, nil)
	if _, err := s.GetQuota("user", old.ID, "day"); err != ErrNotFound {
		t.Errorf("promoted user still has default quota")
	}
	role = RoleMember
	s.UpdateUserFields(old.ID, nil, &role, nil, nil, nil)
	if q, err := s.GetQuota("user", old.ID, "day"); err != nil || q.Source != "default" {
		t.Errorf("demoted user lacks default quota: %+v %v", q, err)
	}

	// hapus default → baris default hilang, khusus tetap
	s.PutQuota("user", baru.ID, "month", 1, 0, 0)
	if _, err := s.SetDefaultQuota(DefaultQuota{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetQuota("user", old.ID, "day"); err != ErrNotFound {
		t.Errorf("default rows must be removed")
	}
	if q, err := s.GetQuota("user", baru.ID, "month"); err != nil || q.Source != "custom" {
		t.Errorf("custom row must stay: %+v %v", q, err)
	}
}
