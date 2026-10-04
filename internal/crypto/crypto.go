// Package crypto menyediakan primitif keamanan JenderalRouter:
// enkripsi secret provider (AES-256-GCM), hash password (Argon2id),
// dan generator/hash API key pengguna (format jr-, SHA-256).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	apiKeyRandomLen = 30 // 30 byte acak → 40 karakter base32 tanpa padding
	apiKeyPrefix    = "jr-"
)

// ErrInvalidCipher dipicu bila ciphertext rusak/tidak sah.
var ErrInvalidCipher = errors.New("ciphertext tidak valid")

// Encrypt mengenkripsi plaintext dengan AES-256-GCM; hasil berformat
// "v1:<nonce-b64>:<ciphertext-b64>" agar mudah diversionkan.
func Encrypt(masterKey, plaintext []byte) (string, error) {
	if len(masterKey) != 32 {
		return "", fmt.Errorf("master key harus 32 byte, dapat %d", len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	return "v1:" + base64.RawStdEncoding.EncodeToString(nonce) + ":" + base64.RawStdEncoding.EncodeToString(ct), nil
}

// Decrypt membuka ciphertext hasil Encrypt.
func Decrypt(masterKey []byte, encoded string) ([]byte, error) {
	parts := strings.Split(encoded, ":")
	if len(parts) != 3 || parts[0] != "v1" {
		return nil, ErrInvalidCipher
	}
	nonce, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidCipher
	}
	ct, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidCipher
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, ErrInvalidCipher
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrInvalidCipher
	}
	return pt, nil
}

// argon2 parameter (OWASP-recommended untuk interaksi login).
const (
	argonMemory  = 64 * 1024 // 64 MiB
	argonTime    = 2
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword menghash password dengan Argon2id dalam format PHC string.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword memverifikasi password terhadap hash PHC Argon2id;
// aman terhadap timing attack (constant-time compare).
func VerifyPassword(phc, password string) bool {
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// NewAPIKey menghasilkan API key pengguna baru: plaintext "jr-" + 40
// karakter alfanumerik, dan hash SHA-256 hex untuk disimpan (FR-4.2).
func NewAPIKey() (plain, hash string, err error) {
	raw := make([]byte, apiKeyRandomLen)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	plain = apiKeyPrefix + base32HexNoPad(raw, 40)
	return plain, HashToken(plain), nil
}

// base32HexNoPad mengubah byte acak menjadi persis n karakter dari
// alfabet [0-9a-v] (base32 hex, tanpa padding) — aman untuk URL/header.
func base32HexNoPad(b []byte, n int) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuv"
	out := make([]byte, 0, n)
	var buf uint64
	var bits uint
	for len(out) < n {
		if bits < 5 && len(b) > 0 {
			buf = buf<<8 | uint64(b[0])
			bits += 8
			b = b[1:]
			continue
		}
		if bits < 5 {
			buf <<= 5
			bits += 5
		}
		idx := (buf >> (bits - 5)) & 0x1f
		bits -= 5
		out = append(out, alphabet[idx])
	}
	return string(out)
}

// HashToken menghitung SHA-256 hex dari token/key (untuk penyimpanan).
func HashToken(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// NewToken menghasilkan token sesi acak (32 byte hex, 64 char).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// PasswordStrength menilai kekuatan password untuk wizard instalasi (NFR-02):
// minimal 12 karakter, mengandung huruf kecil, huruf besar, dan angka.
func PasswordStrength(pw string) error {
	if len(pw) < 12 {
		return errors.New("password minimal 12 karakter")
	}
	var lower, upper, digit bool
	for _, r := range pw {
		switch {
		case 'a' <= r && r <= 'z':
			lower = true
		case 'A' <= r && r <= 'Z':
			upper = true
		case '0' <= r && r <= '9':
			digit = true
		}
	}
	if !lower || !upper || !digit {
		return errors.New("password harus memuat huruf kecil, huruf besar, dan angka")
	}
	return nil
}
