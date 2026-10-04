package store

import (
	"strings"
	"testing"
	"time"
)

func seedProvider(t *testing.T, s *Store) *Provider {
	t.Helper()
	p, err := s.CreateProvider(ProviderOpenAICompat, "Mock", "mk", "http://127.0.0.1:9/v1", CredentialSettings{Strategy: CredStrategyRoundRobin}, true)
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	return p
}

func TestProviderCRUD(t *testing.T) {
	s := newTestStore(t)
	p := seedProvider(t, s)

	got, err := s.GetProvider(p.ID)
	if err != nil || got.Name != "Mock" {
		t.Fatalf("GetProvider: %v", err)
	}

	newURL := "http://127.0.0.1:10/v1"
	if err := s.UpdateProviderFields(p.ID, nil, &newURL, nil, boolPtr(false)); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = s.GetProvider(p.ID)
	if got.BaseURL != newURL || got.Enabled {
		t.Errorf("update tidak tersimpan: %+v", got)
	}
	if err := s.DeleteProvider(p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.GetProvider(p.ID); err != ErrNotFound {
		t.Errorf("harus ErrNotFound: %v", err)
	}
}

func TestDuplicatePrefixRejected(t *testing.T) {
	s := newTestStore(t)
	seedProvider(t, s)
	if _, err := s.CreateProvider(ProviderOpenAICompat, "Lain", "mk", "http://x", CredentialSettings{}, true); err == nil {
		t.Fatal("prefix duplikat harus ditolak")
	}
}

func TestCredentialSelectionStrategies(t *testing.T) {
	s := newTestStore(t)
	p := seedProvider(t, s)
	c1, _ := s.AddCredential(p.ID, "satu", "sk-1", 1)
	c2, _ := s.AddCredential(p.ID, "dua", "sk-2", 1)
	c3, _ := s.AddCredential(p.ID, "tiga", "sk-3", 1)

	// secret tersimpan terenkripsi
	if strings.Contains(c1.SecretEnc, "sk-1") {
		t.Fatal("secret tidak boleh plaintext")
	}
	dec, err := s.DecryptSecret(c1.SecretEnc)
	if err != nil || dec != "sk-1" {
		t.Fatalf("decrypt: %v %q", err, dec)
	}

	// round-robin: berputar
	seen := map[int64]bool{}
	for i := 0; i < 3; i++ {
		c, err := s.PickCredential(p.ID, CredStrategyRoundRobin, nil)
		if err != nil {
			t.Fatalf("pick: %v", err)
		}
		seen[c.ID] = true
	}
	if len(seen) < 2 {
		t.Errorf("round-robin harus berputar, hanya lihat %v", seen)
	}

	// least-used
	s.MarkCredentialSuccess(c1.ID)
	s.MarkCredentialSuccess(c1.ID)
	c, _ := s.PickCredential(p.ID, CredStrategyLeastUsed, nil)
	if c.ID == c1.ID {
		t.Error("least-used harus pilih yang paling jarang dipakai")
	}

	// priority → weight tertinggi
	c4, _ := s.AddCredential(p.ID, "utama", "sk-4", 10)
	c, _ = s.PickCredential(p.ID, CredStrategyPriority, nil)
	if c.ID != c4.ID {
		t.Errorf("priority harus weight tertinggi: dapat %d", c.ID)
	}

	// skip credential tertentu
	c, err = s.PickCredential(p.ID, CredStrategyRoundRobin, map[int64]bool{c1.ID: true, c2.ID: true, c3.ID: true, c4.ID: true})
	if err != ErrNoCredential || c != nil {
		t.Errorf("semua di-skip harus ErrNoCredential: %v %v", c, err)
	}
}

func TestCredentialCooldownAndInvalid(t *testing.T) {
	s := newTestStore(t)
	p := seedProvider(t, s)
	c, _ := s.AddCredential(p.ID, "kunci", "sk-x", 1)

	// 429 pertama → cooldown 60 detik
	if err := s.MarkCredentialFailure(c.ID, 429, "rate limited"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetCredential(c.ID)
	if got.Status != "active" {
		t.Errorf("status = %s", got.Status)
	}
	if _, err := s.PickCredential(p.ID, CredStrategyRoundRobin, nil); err != ErrNoCredential {
		t.Errorf("credential cooldown harus dilewati: %v", err)
	}
	if !got.CooldownUntil.After(time.Now()) {
		t.Errorf("cooldown_until = %v", got.CooldownUntil)
	}

	// cooldown habis → dipakai lagi
	s.ClearCredentialCooldown(c.ID)
	if _, err := s.PickCredential(p.ID, CredStrategyRoundRobin, nil); err != nil {
		t.Errorf("setelah clear harus bisa: %v", err)
	}

	// 401 berulang → invalid permanen
	s.MarkCredentialFailure(c.ID, 401, "wrong key")
	s.ClearCredentialCooldown(c.ID)
	s.MarkCredentialFailure(c.ID, 401, "wrong key again")
	got, _ = s.GetCredential(c.ID)
	if got.Status != "invalid" {
		t.Errorf("401 berulang harus invalid, status = %s", got.Status)
	}
	if _, err := s.PickCredential(p.ID, CredStrategyRoundRobin, nil); err != ErrNoCredential {
		t.Errorf("invalid harus dilewati: %v", err)
	}
}

func TestCredentialCooldownEscalation(t *testing.T) {
	s := newTestStore(t)
	p := seedProvider(t, s)
	c, _ := s.AddCredential(p.ID, "k", "sk", 1)
	var first, prev time.Time
	for i := 0; i < 5; i++ {
		s.MarkCredentialFailure(c.ID, 429, "rl")
		got, _ := s.GetCredential(c.ID)
		if i == 0 {
			first = got.CooldownUntil
			prev = first
		} else {
			if !got.CooldownUntil.After(prev) {
				t.Fatalf("cooldown harus naik eksponensial pada iterasi %d: %v <= %v", i, got.CooldownUntil, prev)
			}
			prev = got.CooldownUntil
		}
		s.ClearCredentialCooldown(c.ID)
		// fail_count bertahan agar eskalasi
		s.DB.Exec(`UPDATE credentials SET cooldown_until='' WHERE id=?`, c.ID)
	}
	if prev.Sub(first.UTC().UTC()) <= 0 {
		t.Log("interval pertama vs terakhir", first, prev)
	}
	// maksimal 15 menit
	for i := 0; i < 10; i++ {
		s.MarkCredentialFailure(c.ID, 429, "rl")
	}
	got, _ := s.GetCredential(c.ID)
	if got.CooldownUntil.Sub(time.Now()) > 16*time.Minute {
		t.Errorf("cooldown melebihi 15 menit: %v", got.CooldownUntil.Sub(time.Now()))
	}
}

func TestModelCRUDAndLookup(t *testing.T) {
	s := newTestStore(t)
	p := seedProvider(t, s)
	m, err := s.CreateModel(p.ID, "gpt-x", "cepat", "GPT X", 1.5, 6, 128000, ModelCap{Tools: true, Vision: true}, true)
	if err != nil {
		t.Fatalf("CreateModel: %v", err)
	}
	if m.PublicID != "mk/gpt-x" {
		t.Errorf("public_id = %s", m.PublicID)
	}
	if m.ProviderName != "Mock" || m.ProviderPrefix != "mk" {
		t.Errorf("join provider kurang: %+v", m)
	}

	byPublic, err := s.FindModelByPublicOrAlias("mk/gpt-x")
	if err != nil || byPublic.ID != m.ID {
		t.Fatalf("lookup public_id: %v", err)
	}
	byAlias, err := s.FindModelByPublicOrAlias("cepat")
	if err != nil || byAlias.ID != m.ID {
		t.Fatalf("lookup alias: %v", err)
	}
	if _, err := s.FindModelByPublicOrAlias("tak/ada"); err != ErrNotFound {
		t.Errorf("harus ErrNotFound: %v", err)
	}

	newPrice := 2.0
	if err := s.UpdateModelFields(m.ID, nil, nil, &newPrice, nil, nil, nil, boolPtr(false)); err != nil {
		t.Fatal(err)
	}
	m, _ = s.GetModel(m.ID)
	if m.PriceInPer1M != 2.0 || m.Enabled {
		t.Errorf("update model tidak tersimpan: %+v", m)
	}
}

func TestUpsertModelSync(t *testing.T) {
	s := newTestStore(t)
	p := seedProvider(t, s)
	if err := s.UpsertModelSinkron(p.ID, "baru-1", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertModelSinkron(p.ID, "baru-1", 0); err != nil {
		t.Fatal(err)
	}
	models, _ := s.ListModels()
	count := 0
	for _, m := range models {
		if m.UpstreamName == "baru-1" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("upsert menduplikat model: %d", count)
	}
}

func TestComboCRUD(t *testing.T) {
	s := newTestStore(t)
	p := seedProvider(t, s)
	m1, _ := s.CreateModel(p.ID, "m1", "", "", 0, 0, 0, ModelCap{}, true)
	m2, _ := s.CreateModel(p.ID, "m2", "", "", 0, 0, 0, ModelCap{}, true)

	c, err := s.CreateCombo("coding-hemat", "Sonnet → lokal", []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("CreateCombo: %v", err)
	}
	if len(c.Steps) != 2 || c.Steps[0].Position != 1 || c.Steps[0].ModelID != m1.ID {
		t.Fatalf("steps = %+v", c.Steps)
	}

	byName, err := s.GetComboByName("coding-hemat")
	if err != nil || byName.ID != c.ID {
		t.Fatalf("GetComboByName: %v", err)
	}

	newName := "hemat-v2"
	if err := s.UpdateCombo(c.ID, &newName, nil, []int64{m2.ID}); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCombo(c.ID)
	if c.Name != "hemat-v2" || len(c.Steps) != 1 {
		t.Errorf("update combo: %+v", c)
	}

	if err := s.DeleteCombo(c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCombo(c.ID); err != ErrNotFound {
		t.Errorf("combo harus terhapus: %v", err)
	}
}

func TestQuotaLifecycle(t *testing.T) {
	s := newTestStore(t)
	q, err := s.PutQuota("user", 1, "day", 100000, 500, 2.5)
	if err != nil {
		t.Fatalf("PutQuota: %v", err)
	}
	if q.TokenLimit != 100000 {
		t.Errorf("token_limit = %d", q.TokenLimit)
	}
	// upsert mengubah limit, mempertahankan used
	s.RecordQuotaUsage("user", 1, 1000, 0.5)
	q, _ = s.GetQuota("user", 1, "day")
	if q.UsedTokens != 1000 || q.UsedRequests != 1 {
		t.Fatalf("used = %+v", q)
	}
	q, err = s.PutQuota("user", 1, "day", 200000, 1000, 5)
	if err != nil || q.TokenLimit != 200000 {
		t.Fatalf("upsert quota: %v %+v", err, q)
	}
	q, _ = s.GetQuota("user", 1, "day")
	if q.UsedTokens != 1000 {
		t.Errorf("used harus bertahan: %+v", q)
	}
	// reset
	if err := s.ResetQuota(q.ID, time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	q, _ = s.GetQuota("user", 1, "day")
	if q.UsedTokens != 0 || q.ResetAt == "" {
		t.Errorf("reset gagal: %+v", q)
	}
}

func TestQuotasDueReset(t *testing.T) {
	s := newTestStore(t)
	q, _ := s.PutQuota("user", 7, "day", 100, 0, 0)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	s.DB.Exec(`UPDATE quotas SET reset_at=? WHERE id=?`, past, q.ID)
	due, err := s.QuotasDueReset(time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range due {
		if d.ID == q.ID {
			found = true
		}
	}
	if !found {
		t.Error("kuota jatuh tempo harus terdeteksi")
	}
}

func TestProviderTemplatesComplete(t *testing.T) {
	if len(ProviderTemplates) != 12 {
		t.Fatalf("template = %d, mau 12 (FR-1.1)", len(ProviderTemplates))
	}
	prefixes := map[string]bool{}
	for _, tp := range ProviderTemplates {
		if tp.Prefix == "" || tp.Name == "" || tp.BaseURL == "" {
			t.Errorf("template tidak lengkap: %+v", tp.Name)
		}
		if prefixes[tp.Prefix] {
			t.Errorf("prefix duplikat: %s", tp.Prefix)
		}
		prefixes[tp.Prefix] = true
	}
	local := FindTemplate("local")
	if local == nil || local.Type != ProviderLlamaStash {
		t.Fatal("template LlamaStash harus ada")
	}
	if local.BaseURL != "http://127.0.0.1:11435/v1" {
		t.Errorf("base url lokal = %s (FR-6.1)", local.BaseURL)
	}
	if FindTemplate("tidak-ada") != nil {
		t.Error("template tak dikenal harus nil")
	}
}

func TestSeedProvidersFromTemplate(t *testing.T) {
	s := newTestStore(t)
	tp := FindTemplate("an")
	p, err := s.SeedProvidersFromTemplate(tp)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	models, _ := s.ListModels()
	if len(models) != len(tp.Models) {
		t.Fatalf("model = %d, mau %d", len(models), len(tp.Models))
	}
	for _, m := range models {
		if m.ProviderID != p.ID || m.PublicID[:3] != "an/" {
			t.Errorf("model salah: %+v", m.PublicID)
		}
	}
}

func boolPtr(b bool) *bool { return &b }
