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

## Instalasi di Server Ubuntu (22.04 / 24.04 / 26.04)

### 0. Persiapan VPS (sekali di awal)

Login ke VPS dan siapkan dependensi dasar:

```bash
ssh root@IP-VPS-ANDA

sudo apt update && sudo apt -y upgrade
sudo apt -y install curl git ca-certificates
```

Spesifikasi minimum: **1 vCPU / 1 GB RAM** (hanya provider cloud); disarankan **2 vCPU / 4 GB** (bisa jalankan model lokal kecil).

---

### Cara A — Skrip otomatis (paling cepat)

Skrip `install-ubuntu.sh` mendeteksi versi Ubuntu otomatis, membuat user sistem,
memasang service systemd, dan mengatur firewall (hanya 80/443 ke publik).
**Port SSH Anda dideteksi otomatis** (termasuk yang bukan 22) dan diizinkan di
UFW agar sesi SSH tidak terputus — bila mode `binary`, skrip juga otomatis
build dari source bila rilis binary belum tersedia:

```bash
git clone https://github.com/mohammadirham37/jenderal_router.git
cd jenderal_router

sudo ./deploy/install-ubuntu.sh binary    # mode binary + systemd (tanpa Docker)
# atau
sudo ./deploy/install-ubuntu.sh docker    # mode Docker Compose + Caddy (HTTPS otomatis)
```

Setelah selesai, lanjut ke **Akses pertama** di bawah.

---

### Cara B — Manual: binary + systemd (paling ringan; tanpa Docker)

**1) Dapatkan binary.** Unduh dari Releases (jika sudah tersedia), atau build dari source:

```bash
# opsi 1: unduh binary rilis (linux amd64)
curl -fSL -o jr.tar.gz https://github.com/mohammadirham37/jenderal_router/releases/latest/download/jenderalrouter-linux-amd64.tar.gz
tar -xzf jr.tar.gz                       # menghasilkan file: jenderalrouter-linux-amd64
BIN=jenderalrouter-linux-amd64

# opsi 2: build dari source (butuh Go 1.23+, tanpa CGO)
git clone https://github.com/mohammadirham37/jenderal_router.git
cd jenderal_router && ./scripts/build.sh && cd ..
BIN=jenderal_router/dist/jenderalrouter-linux-amd64
```

> Punya VPS ARM (mis. Oracle/Ampere)? Ganti `amd64` menjadi `arm64` di semua perintah.

**2) Pasang binary + user sistem + direktori data:**

```bash
sudo install -m 0755 "$BIN" /usr/local/bin/jenderalrouter

sudo useradd --system --home /var/lib/jenderalrouter --shell /usr/sbin/nologin jenderalrouter
sudo mkdir -p /var/lib/jenderalrouter
sudo chown jenderalrouter:jenderalrouter /var/lib/jenderalrouter
sudo chmod 700 /var/lib/jenderalrouter
```

**3) File konfigurasi environment:**

```bash
sudo tee /etc/default/jenderalrouter >/dev/null <<'ENV'
JR_ADDR=127.0.0.1:20130
JR_DATA_DIR=/var/lib/jenderalrouter
JR_TZ=Asia/Jakarta
JR_LOG_PROMPTS=false
# Opsional:
# JR_PUBLIC_URL=https://ai.domainanda.com
# JR_LLAMASTASH_URL=http://127.0.0.1:11435/v1
ENV
sudo chmod 600 /etc/default/jenderalrouter
```

**4) Service systemd:**

```bash
sudo curl -fSL -o /etc/systemd/system/jenderalrouter.service \
  https://raw.githubusercontent.com/mohammadirham37/jenderal_router/main/deploy/jenderalrouter.service
# (bila build dari source: sudo cp jenderal_router/deploy/jenderalrouter.service /etc/systemd/system/)

sudo systemctl daemon-reload
sudo systemctl enable --now jenderalrouter
sudo systemctl status jenderalrouter --no-pager
```

**5) Firewall** — hanya 80/443 yang menghadap publik; port aplikasi tetap loopback:

```bash
sudo ufw allow OpenSSH
sudo ufw allow 80,443/tcp
sudo ufw enable
```

**6) HTTPS / akses LAN dengan Caddy** — lihat bagian **Opsi B+ — Caddy** di
bawah: varian (a) untuk akses via IP LAN, varian (b) untuk HTTPS domain.
Tanpa keduanya, akses via SSH tunnel (sesuaikan `-p` bila SSH bukan port 22):

```bash
ssh -p PORT_SSH_ANDA -L 20130:127.0.0.1:20130 root@IP-VPS-ANDA
# lalu buka http://localhost:20130 di browser
```

---

### Cara C — Docker Compose + Caddy (HTTPS otomatis)

```bash
sudo apt -y install ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo $VERSION_CODENAME) stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
sudo apt update && sudo apt -y install docker-ce docker-ce-cli containerd.io docker-compose-plugin

git clone https://github.com/mohammadirham37/jenderal_router.git
cd jenderal_router/deploy
cp .env.example .env
nano .env                       # isi JR_DOMAIN=ai.domainanda.com
sudo docker compose up -d --build
```

---

### Opsi B+ — Caddy: akses mudah via IP LAN atau domain (disarankan)

Setelah aplikasi berjalan (Cara A/B/C), pasang Caddy supaya dashboard bisa
dibuka dari PC lain **tanpa SSH tunnel**. Aplikasi tetap listen di loopback —
hanya Caddy yang menghadap jaringan.

**a) Tanpa domain — akses via IP di jaringan lokal:**

```bash
sudo apt -y install caddy

sudo tee /etc/caddy/Caddyfile >/dev/null <<'CADDY'
:80 {
    reverse_proxy 127.0.0.1:20130 {
        flush_interval -1
    }
}
CADDY

sudo systemctl reload caddy
```

Akses dari PC lain: `http://<ip-server>/` — contoh `http://172.26.8.104/`.
Port 20130 tetap tertutup dari luar (UFW hanya membuka 80/443).

**b) Dengan domain — HTTPS otomatis (Let's Encrypt):**

```bash
sudo tee /etc/caddy/Caddyfile >/dev/null <<'CADDY'
ai.domainanda.com {
    reverse_proxy 127.0.0.1:20130 {
        flush_interval -1
    }
}
CADDY

sudo systemctl reload caddy
```

Akses: `https://ai.domainanda.com/`. Syarat: A record domain mengarah ke IP
server. `flush_interval -1` wajib agar streaming SSE tidak ter-buffer.

Alternatif tanpa Caddy sama sekali: SSH tunnel
`ssh -p PORT_SSH_ANDA -L 20130:127.0.0.1:20130 user@ip-server` lalu buka
`http://localhost:20130`.

### Akses pertama

Buka `https://domain-anda.com` (atau `http://localhost:20130` via tunnel).
**Wizard pertama** meminta pembuatan akun super admin — tidak ada password
bawaan (minimal 12 karakter, campuran huruf besar/kecil/angka).
Setelah masuk: tambah provider (menu **Provider → Dari Template**), buat API
key (menu **User & Peran**), lalu pakai di klien.

### Akses dari PC lain di jaringan lokal (LAN)?

Gunakan **Opsi B+ — Caddy** di atas (varian a: `:80`), atau publikasikan port
aplikasi ke subnet LAN — panduan lengkap:
[docs/OPERATIONS.md §8](docs/OPERATIONS.md).

### Verifikasi instalasi

```bash
curl http://127.0.0.1:20130/healthz     # {"status":"ok","version":"..."}
curl http://127.0.0.1:20130/readyz      # {"status":"ready"}
```

Berlaku untuk mode binary maupun Docker (port 20130 hanya di-publish ke
loopback host — publik tetap lewat Caddy). Jika kedua perintah menjawab JSON,
gateway siap dipakai.

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

Di dashboard: **Provider → Dari Template → LlamaStash (lokal)** — atau sekalian dari halaman **Status LlamaStash → Install LlamaStash** (mengunduh installer resmi, menjalankan `init --recommended --json`, mendaftarkan provider + model otomatis, dengan log progres live).

Integrasi mengikuti kontrak [llamastash/llamastash](https://github.com/llamastash/llamastash) (diverifikasi terhadap v0.6.1): proxy OpenAI-compatible + Anthropic-native di `127.0.0.1:11435` (auto-start model by name, auto-fallback antar model), discovery via `GET /v1/models` + `/health`, status real-time via `llamastash status --json` (proxy.listen, auth, CPU/RAM/GPU, launch berjalan), start/stop via CLI (`start <ref> --json`), dan bearer key via `llamastash api-key` (loopback keyless — key hanya wajib pada mode LAN).
Konkurensi default 2 request/model, antre 30 detik, cold start hingga 120 detik (keep-alive SSE dikirim ke klien saat menunggu). Halaman **Status LlamaStash** menampilkan daemon, proxy listen, statistik host, daftar model (dimuat/ditemukan), dan tombol start/stop.

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

UI dibangun dengan **SvelteKit + TypeScript + ikon Lucide** (folder `web/`,
adapter-static SPA) lalu di-embed ke binary via `embed.FS`. Hasil build UI
**ikut ter-commit** di `internal/web/dist`, sehingga build Go murni tidak
membutuhkan Node; bila Node tersedia, `scripts/build.sh` otomatis membangun
ulang UI dari `web/` terlebih dahulu.

## Arsitektur singkat

```
Klien (SDK/CLI/browser)
   │  Bearer jr-…
   ▼
Caddy (HTTPS) ──► JenderalRouter (satu binary Go, UI SvelteKit ter-embed)
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
