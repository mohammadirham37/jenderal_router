package crypto

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	msg := []byte(`sk-rahasia-provider-1234567890`)
	enc, err := Encrypt(key, msg)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := Decrypt(key, enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Errorf("round-trip berbeda: %q != %q", got, msg)
	}
	// nonce acak: dua enkripsi pesan sama harus menghasilkan ciphertext beda
	enc2, _ := Encrypt(key, msg)
	if enc == enc2 {
		t.Error("nonce harus acak; ciphertext identik terdeteksi")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	enc, _ := Encrypt(bytes.Repeat([]byte{1}, 32), []byte("data"))
	if _, err := Decrypt(bytes.Repeat([]byte{2}, 32), enc); err == nil {
		t.Fatal("kunci salah harus gagal")
	}
	if _, err := Decrypt(bytes.Repeat([]byte{1}, 32), "v1:xx:yy"); err == nil {
		t.Fatal("ciphertext sampah harus gagal")
	}
	if _, err := Encrypt([]byte("pendek"), []byte("x")); err == nil {
		t.Fatal("master key <32 byte harus ditolak")
	}
}

func TestPasswordHashVerify(t *testing.T) {
	h, err := HashPassword("Sandi-Kuat-123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Errorf("format PHC salah: %q", h[:20])
	}
	if !VerifyPassword(h, "Sandi-Kuat-123") {
		t.Error("password benar harus terverifikasi")
	}
	if VerifyPassword(h, "sandi-salah") {
		t.Error("password salah tidak boleh lolos")
	}
	// hash kedua untuk password sama harus beda (salt acak)
	h2, _ := HashPassword("Sandi-Kuat-123")
	if h == h2 {
		t.Error("salt harus acak")
	}
	// hash rusak tidak boleh panic
	if VerifyPassword("rusak", "x") {
		t.Error("hash rusak harus false")
	}
}

func TestNewAPIKey(t *testing.T) {
	plain, hash, err := NewAPIKey()
	if err != nil {
		t.Fatalf("NewAPIKey: %v", err)
	}
	if !strings.HasPrefix(plain, "jr-") {
		t.Errorf("prefix = %q", plain[:3])
	}
	if len(plain) != len("jr-")+40 {
		t.Errorf("panjang key = %d, mau %d", len(plain), len("jr-")+40)
	}
	for _, c := range plain[3:] {
		if !strings.ContainsRune("0123456789abcdefghijklmnopqrstuv", c) {
			t.Errorf("karakter ilegal %q", c)
		}
	}
	if hash != HashToken(plain) {
		t.Error("hash harus SHA-256 dari plaintext")
	}
	p2, _, _ := NewAPIKey()
	if p2 == plain {
		t.Error("key harus unik")
	}
}

func TestPasswordStrength(t *testing.T) {
	cases := []struct {
		pw    string
		valid bool
	}{
		{"Pendek1a", false},              // < 12
		{"hanya-huruf-kecil-aja", false}, // tanpa besar/angka
		{"HURUF BESAR SEMUA 123", false},
		{"HurufBesarTanpaAngka", false},
		{"Sandi-Kuat-123", true},
		{"jenderal2026X", true},
	}
	for _, c := range cases {
		err := PasswordStrength(c.pw)
		if c.valid && err != nil {
			t.Errorf("%q harus valid: %v", c.pw, err)
		}
		if !c.valid && err == nil {
			t.Errorf("%q harus ditolak", c.pw)
		}
	}
}
