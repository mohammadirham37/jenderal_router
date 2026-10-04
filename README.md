# JenderalRouter 🛰️

**Gateway LLM self-hosted multi-user** — satu endpoint API kompatibel OpenAI & Anthropic, satu dashboard GUI, banyak provider di belakangnya, termasuk model lokal via [LlamaStash](https://llamastash.dev/).

Terinspirasi [9Router](https://github.com/decolua/9router), dirancang **multi-user sejak awal**: akun & peran, API key per user, kuota per user/key, combo model dengan fallback otomatis, dan analitik terpusat.

## Fitur MVP

| Fitur | Keterangan |
| --- | --- |
| 🔌 12+ provider | OpenAI, Anthropic, Gemini, OpenRouter, Groq, DeepSeek, Mistral, Together, GLM, MiniMax, Kimi, **LlamaStash (lokal)** + provider kustom |
| 🌐 API kompatibel | `POST /v1/chat/completions` (OpenAI), `POST /v1/messages` (Anthropic — siap untuk Claude Code), `GET /v1/models`, `GET /v1/usage/me` |
| 🔄 Combo + fallback | Rantai model berurutan; fallback saat 429/5xx/timeout/kuota habis, retry credential lain; circuit breaker per provider |
| 🗝️ Multi-key per provider | Round-robin / priority / least-used, cooldown eksponensial saat 429 (60 s → maks 15 menit), key invalid saat 401 berulang |
| 👥 Multi-user & RBAC | `super_admin`, `admin`, `member`, `viewer`; API key `jr-…` (hash SHA-256, tampil sekali) |
| 📊 Kuota & rate limit | Token/request/biaya per hari & bulan (zona waktu bisa diatur, default Asia/Jakarta), RPM/TPM per key, IP allowlist |
| 📈 Log & analitik | Log request asinkron (tanpa isi prompt secara default), estimasi biaya, grafik, ekspor CSV, `/metrics` Prometheus |
| 🖥️ Dashboard + Chat | Admin dashboard lengkap + chat playground dengan streaming, riwayat, markdown, dark/light, ID/EN |
| 🔒 Keamanan | Argon2id, AES-256-GCM untuk secret provider, cookie HttpOnly/Secure/SameSite=Strict + CSRF, rate limit login, SSRF guard, wizard tanpa password bawaan |

## Instalasi ≤ 10 menit (Ubuntu 22.04 / 24.04 / 26.04)

### Cara A — binary + systemd (paling ringan; 1 vCPU / 1 GB cukup)

```bash
wget https://github.com/jenderal/jenderalrouter/releases/latest/download/jenderalrouter-linux-amd64.tar.gz
tar -xzf jenderalrouter-linux-amd64.tar.gz
sudo install -m 0755 jenderalrouter /usr/local/bin/
sudo install -d -o root -g root /etc/default

# service systemd (lihat deploy/jenderalrouter.service)
sudo cp deploy/jenderalrouter.service /etc/systemd/system/
sudo useradd --system --home /var/lib/jenderalrouter --shell /usr/sbin/nologin jenderalrouter || true
sudo mkdir -p /var/lib/jenderalrouter && sudo chown jenderalrouter: /var/lib/jenderalrouter
sudo systemctl daemon-reload && sudo systemctl enable --now jenderalrouter
```

Atau sekali jalan: `sudo ./deploy/install-ubuntu.sh binary`

### Cara B — Docker Compose + Caddy (HTTPS otomatis)

```bash
git clone https://github.com/jenderal/jenderalrouter && cd jenderalrouter/deploy
cp .env.example .env   # isi JR_DOMAIN=ai.domainanda.com
docker compose up -d --build
```

Atau: `sudo ./deploy/install-ubuntu.sh docker`

### Akses pertama

Buka `http://127.0.0.1:20130` (SSH tunnel: `ssh -L 20130:127.0.0.1:20130 vps`) atau domain Anda.
**Wizard pertama** meminta pembuatan akun super admin — tidak ada password bawaan (minimal 12 karakter, campuran huruf besar/kecil/angka).

### HTTPS dengan Caddy (tanpa Docker)

```caddyfile
ai.domainanda.com {
    reverse_proxy 127.0.0.1:20130 {
        flush_interval -1   # wajib untuk streaming SSE
    }
}
```

Port publik hanya **80/443**. Port aplikasi (20130) dan LlamaStash (11435) tetap loopback.

## Pakai dari klien

### OpenAI SDK

```python
from openai import OpenAI
client = OpenAI(base_url="https://ai.domainanda.com/v1", api_key="jr-xxxx")
r = client.chat.completions.create(model="oa/gpt-5.4",
    messages=[{"role": "user", "content": "Halo"}])
```

### Anthropic SDK / Claude Code

```bash
export ANTHROPIC_BASE_URL=https://ai.domainanda.com
export ANTHROPIC_API_KEY=jr-xxxx        # key JenderalRouter dipakai apa adanya
claude                                  # semua request lewat JenderalRouter
```

### curl + streaming + combo fallback

```bash
curl https://ai.domainanda.com/v1/chat/completions \
  -H "Authorization: Bearer jr-xxxx" -H "Content-Type: application/json" \
  -d '{"model": "coding-hemat", "stream": true,
       "messages": [{"role": "user", "content": "Halo"}]}'
```

Header respons `X-Route-Provider`, `X-Route-Model`, `X-Route-Attempts` menunjukkan provider/model aktual.

### Kode error

| HTTP | Kode | Arti |
| --- | --- | --- |
| 401 | `invalid_api_key` | Key salah/cabut/kedaluwarsa |
| 403 | `model_not_allowed` | Model tidak diizinkan untuk key |
| 404 | `model_not_found` | Model/combo tidak ada |
| 429 | `quota_exceeded` / `rate_limited` | Kuota/RPM/TPM habis (`Retry-After`) |
| 502 | `upstream_error` | Semua langkah combo gagal (rincian di `error.attempts`) |
| 504 | `upstream_timeout` | Provider tidak merespons |

## LlamaStash (model lokal, biaya 0)

```bash
# di host VPS (bukan container):
curl -fsSL https://llamastash.dev/install.sh | sh
llamastash init --recommended --json     # setup tanpa interaksi
```

Di dashboard: **Provider → Dari Template → LlamaStash (lokal)**.
Model lokal otomatis ditemukan via `GET /v1/models` proxy (fallback: CLI `llamastash list --json`).
Konkurensi default 2 request/model, antre 30 detik, cold start hingga 120 detik (keep-alive SSE dikirim ke klien saat menunggu). Halaman **Status LlamaStash** menampilkan daemon, model, dan tombol start/stop (via CLI saat tersedia).

Panduan kapasitas: 4 vCPU/8 GB → model 1–4B Q4 · 8 vCPU/16 GB → 7–8B Q4 · GPU 24 GB → 14–32B Q4.
Tanpa GPU, jadikan model lokal **langkah terakhir combo**.

## Konfigurasi (env)

| Variabel | Default | Keterangan |
| --- | --- | --- |
| `JR_ADDR` | `127.0.0.1:20130` | Alamat bind (set `0.0.0.0:20130` di balik reverse proxy) |
| `JR_DATA_DIR` | `data` | Lokasi DB, master key, backup |
| `JR_DATABASE_URL` | `<data>/jenderalrouter.db` | SQLite (PostgreSQL menyusul v1.1) |
| `JR_MASTER_KEY` | dibuat otomatis (`data/master.key`, 0600) | Kunci AES-256-GCM secret provider (hex 64 atau passphrase) |
| `JR_PUBLIC_URL` | — | `https://…` → cookie Secure otomatis |
| `JR_LLAMASTASH_URL` | `http://127.0.0.1:11435/v1` | Proxy LlamaStash |
| `JR_LOG_PROMPTS` | `false` | Simpan isi prompt untuk debug (jangan di produksi) |
| `JR_TZ` | `Asia/Jakarta` | Zona reset kuota harian/bulanan |
| `JR_BACKUP_ENABLED` / `JR_BACKUP_HOUR` | `true` / `3` | Backup harian otomatis (retensi 7) |
| `JR_SESSION_TTL` | `24h` | Masa berlaku sesi dashboard |

## Operasi

- **Backup**: otomatis harian ke `data/backups/` (retensi 7 salinan) atau manual via dashboard (Pengaturan → Backup).
- **Monitoring**: `/healthz`, `/readyz`, `/metrics` (Prometheus, format 0.0.4).
- **Keamanan**: semua `/v1/*` wajib API key — tidak ada mode tanpa auth. Isi prompt tidak disimpan; log bisa diekspor ke CSV. Lihat [docs/OPERATIONS.md](docs/OPERATIONS.md).

## Build dari source

Butuh Go 1.23+ (dikembangkan dengan 1.27). Tanpa CGO — silang-kompilasi mudah:

```bash
./scripts/build.sh v1.0.0     # vet + test + build linux amd64/arm64 → dist/
```

UI statis (vanilla JS di `internal/api/static/`) ter-embed otomatis via `embed.FS` — satu binary, tanpa toolchain Node.

## Arsitektur singkat

```
Klien (SDK/CLI/browser)
   │  Bearer jr-…
   ▼
Caddy (HTTPS) ──► JenderalRouter (satu binary Go)
                   ├─ auth key + kuota (cache memori)
                   ├─ resolusi model/alias/combo
                   ├─ fallback + retry + circuit breaker
                   ├─ translator OpenAI ⇄ Anthropic ⇄ Gemini
                   ├─ adapter provider (cloud API key / LlamaStash loopback)
                   └─ log & usage asinkron → SQLite (WAL)
```

## Status & roadmap

MVP selesai (F-01…F-15 inti). Berikutnya (v1.1): `/v1/embeddings`, `/v1/responses` (Codex), token saver, cache respons, notifikasi Telegram/email. v1.2: routing berbasis aturan, endpoint gambar/audio, SSO/2FA.

## Lisensi

MIT.
