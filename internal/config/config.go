// Package config memuat konfigurasi JenderalRouter dari variabel environment.
package config

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config adalah seluruh pengaturan runtime. Semua nilai berasal dari env
// dengan prefix JR_ (lihat README bagian konfigurasi).
type Config struct {
	Addr          string // alamat bind aplikasi; default 127.0.0.1:20130
	PublicURL     string // URL publik untuk ditampilkan di UI, mis. https://ai.domain.com
	DataDir       string // direktori data (db, master key, backup)
	DatabaseURL   string // path file sqlite atau "file::memory:?cache=shared"
	RedisURL      string // opsional; MVP belum memakai Redis (dicatat untuk v1.1)
	MasterKey     []byte // kunci AES-256-GCM untuk secret provider (32 byte)
	LLamastashURL string // base URL proxy LlamaStash, default http://127.0.0.1:11435/v1
	LogPrompts    bool   // simpan isi prompt/respons (default false, NFR-14)
	TZ            string // zona waktu reset kuota, default Asia/Jakarta
	CookieSecure  bool   // atribut Secure cookie sesi (otomatis true jika PublicURL https)
	SessionTTL    time.Duration
	BackupEnabled bool
	BackupHour    int // jam UTC/jam-lokal penyimpanan backup harian
	ShmMode       bool
	Environment   string // development|production
}

// Load membaca env dan mengembalikan konfigurasi final.
func Load() (*Config, error) {
	c := &Config{
		Addr:          env("JR_ADDR", "127.0.0.1:20130"),
		PublicURL:     strings.TrimRight(env("JR_PUBLIC_URL", ""), "/"),
		DataDir:       env("JR_DATA_DIR", "data"),
		LLamastashURL: strings.TrimRight(env("JR_LLAMASTASH_URL", "http://127.0.0.1:11435/v1"), "/"),
		LogPrompts:    envBool("JR_LOG_PROMPTS", false),
		TZ:            env("JR_TZ", "Asia/Jakarta"),
		SessionTTL:    envDuration("JR_SESSION_TTL", 24*time.Hour),
		BackupEnabled: envBool("JR_BACKUP_ENABLED", true),
		BackupHour:    envInt("JR_BACKUP_HOUR", 3),
		Environment:   env("JR_ENV", "production"),
	}
	c.DatabaseURL = env("JR_DATABASE_URL", filepath.Join(c.DataDir, "jenderalrouter.db"))
	c.RedisURL = env("JR_REDIS_URL", "")

	if strings.HasPrefix(c.PublicURL, "https://") {
		c.CookieSecure = true
	}
	c.CookieSecure = envBool("JR_COOKIE_SECURE", c.CookieSecure)

	mk, err := loadMasterKey(c.DataDir)
	if err != nil {
		return nil, err
	}
	c.MasterKey = mk
	return c, nil
}

// loadMasterKey mengambil master key dari env JR_MASTER_KEY (hex 64 char atau
// passphrase apa pun yang di-hash); bila kosong, membuat file data/master.key
// dengan izin 0600 (NFR-04).
func loadMasterKey(dataDir string) ([]byte, error) {
	if v := os.Getenv("JR_MASTER_KEY"); v != "" {
		if b, err := hex.DecodeString(v); err == nil && len(b) == 32 {
			return b, nil
		}
		// passphrase bebas: turunkan jadi 32 byte deterministik.
		return deriveKey([]byte(v)), nil
	}
	path := filepath.Join(dataDir, "master.key")
	if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
		return b, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("membuat master key: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("menyimpan master key %s: %w", path, err)
	}
	return key, nil
}

// deriveKey menurunkan kunci 32 byte dari passphrase memakai PBKDF2-SHA256
// (master key hanya tersedia di memori/env; secret provider tetap
// terenkripsi AES-256-GCM).
func deriveKey(pass []byte) []byte {
	key, err := pbkdf2.Key(sha256.New, string(pass), []byte("jenderalrouter/master/v1"), 120_000, 32)
	if err != nil {
		return nil
	}
	return key
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
