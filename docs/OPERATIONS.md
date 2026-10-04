# OPERATIONS — Panduan Operasi JenderalRouter

Panduan harian untuk Super Admin: keamanan, backup/restore, monitoring,
rotasi key, dan pemecahan masalah umum.

## 1. Keamanan produksi

1. **Bind ke loopback.** Biarkan `JR_ADDR=127.0.0.1:20130`; hanya Caddy/Nginx
   yang menghadap publik (80/443).
2. **Master key.** Kunci AES-256-GCM untuk secret provider ada di
   `$JR_DATA_DIR/master.key` (0600). Boleh di-set via `JR_MASTER_KEY`
   (hex 64 char atau passphrase). **Backup file ini** — tanpa dia, secret
   provider tidak bisa dibaca ulang.
3. **Password admin.** Argon2id; kebijakan minimal 12 karakter saat dibuat.
   Ganti berkala via dashboard (User → hapus/buat, atau PATCH password).
4. **Rate limit login**: 5 percobaan / 15 menit / IP (berlaku otomatis).
5. **Audit log**: semua aksi admin tercatat (dashboard → Pengaturan → Audit log,
   atau `audit_logs` di DB).
6. **LlamaStash**: pastikan port 11435 **tidak** dibuka ke publik
   (`ss -tlnp | grep 11435` harus menampilkan `127.0.0.1:11435`).
7. **SSRF guard**: base URL provider kustom yang mengarah ke IP
   privat/loopback/metadata (`169.254.169.254`) diblokir. Bila provider internal
   memang disengaja (mis. vLLM di LAN), centang `ssrf_allow_private` di settings
   provider.
8. **`JR_LOG_PROMPTS`** tetap `false` di produksi; aktifkan hanya sementara
   untuk debugging.

## 2. Backup & restore

### Otomatis
Backup harian berjalan pada jam `JR_BACKUP_HOUR` (default 03:00) ke
`$JR_DATA_DIR/backups/jr-YYYYMMDD-HHMMSS.db`, retensi 7 salinan.
Jalankan manual: dashboard → Pengaturan → **Backup sekarang**, atau
`POST /api/admin/system/backup`.

### Restore
```bash
sudo systemctl stop jenderalrouter
cp /var/lib/jenderalrouter/backups/jr-20261004-030000.db \
   /var/lib/jenderalrouter/jenderalrouter.db
sudo systemctl start jenderalrouter
```
WAL mode: pastikan tidak ada file `jenderalrouter.db-wal` basi saat restore
(hapus `-wal`/`-shm` setelah service berhenti).

## 3. Monitoring

| Endpoint | Isi |
| --- | --- |
| `GET /healthz` | liveness + versi |
| `GET /readyz` | 200 bila DB menjawab |
| `GET /metrics` | Prometheus: `jr_requests_total`, `jr_tokens_total`, `jr_upstream_errors_total`, `jr_request_duration_seconds`, `jr_circuit_state`, `jr_sessions_active` |

Contoh alert Prometheus:

```yaml
- alert: JenderalRouterUpstreamErrors
  expr: rate(jr_upstream_errors_total[5m]) > 0.5
  for: 10m
- alert: JenderalRouterCircuitOpen
  expr: jr_circuit_state == 1
  for: 5m
```

Log aplikasi: JSON ke stdout (journald bila systemd):
`journalctl -u jenderalrouter -f | jq`.

## 4. Rotasi API key provider

1. Dashboard → Provider → tambah credential baru (label + key baru).
2. Tes koneksi → harus `ok`.
3. Hapus credential lama. Round-robin otomatis berhenti memakai key yang
   dihapus; request berjalan tanpa jeda.

Key yang terkena 429 masuk cooldown otomatis (60 dtk, eksponensial hingga
15 menit); 401 berulang menandai `invalid` dan key dihentikan sampai
diganti/diperbaiki.

## 5. Combo: pola yang disarankan

- **Hemat biaya**: model murah → model lokal (biaya 0).
- **Privasi**: model lokal saja untuk user tertentu (batasi `allowed_models`
  pada API key: `local/qwen3-8b` misalnya).
- **Keandalan**: provider primer → provider sekunder (region/akun berbeda).

Perhatian: fallback streaming hanya terjadi **sebelum token pertama**
(FR-2.3); error di tengah stream diteruskan apa adanya.

## 6. Troubleshooting

| Gejala | Sebab & langkah |
| --- | --- |
| 401 `invalid_api_key` padahal key benar | Key dicabut/kedaluwarsa; cache key memori berumur 30 dtk — tunggu sejenak atau buat key baru |
| 429 `quota_exceeded` | Cek kuota user/key (`GET /v1/usage/me`, dashboard User → Kuota); reset otomatis tengah malam `JR_TZ` |
| 502 dengan `attempts` semua cooldown | Semua credential provider cooldown — tambah key atau tunggu backoff selesai |
| Streaming "menggantung" di balik proxy | Pastikan `flush_interval -1` (Caddy) / `proxy_buffering off` (Nginx) |
| Model lokal timeout saat cold start | Wajar untuk GGUF besar (hingga 120 dtk). Klien menerima keep-alive SSE. Bila sering, perbesar RAM atau pakai model lebih kecil |
| `ciphertext tidak valid` saat request | Master key berubah (file `master.key` dihapus/di-replace) — kembalikan master key lama |
| `/v1/messages` ditolak Claude Code | Pastikan `ANTHROPIC_BASE_URL` tanpa `/v1` dan key di `ANTHROPIC_API_KEY` |

## 7. Pembaruan & LlamaStash dari dashboard

### Pembaruan aplikasi (Pengaturan → Pembaruan Aplikasi)

- Tombol **Cek pembaruan**: fetch origin + hitung commit ketinggalan.
- Tombol **Perbarui sekarang** (super_admin): clone/mirror repo ke
  `<data>/src` (milik user service), reset ke `origin/main`, build binary
  baru, smoke test, lalu:
  - **helper sudo aktif** (dipasang `install-ubuntu.sh binary`, jawab Y):
    binary dipasang + service di-restart otomatis.
  - tanpa helper: binary distage di `<data>/updates/jenderalrouter.new`
    dan perintah pemasangan manual ditampilkan.
- Di mode Docker: endpoint hanya menampilkan petunjuk host
  (`git pull && docker compose up -d --build`).
- Risiko yang perlu diketahui: helper sudo mengeksekusi binary hasil build
  service sebagai root — kompromi pada service = kompromi penuh. Bila tidak
  nyaman, jawab `n` saat instalasi atau hapus
  `/etc/sudoers.d/jenderalrouter-update`.

### Install LlamaStash (Status LlamaStash → Install)

- Hanya tersedia di mode binary (aplikasi di host). Di container, halaman
  menampilkan perintah instalasi host.
- Alur: unduh installer resmi → `llamastash init --recommended --json` →
  daftarkan provider "LlamaStash (lokal)" + sinkron model. Bearer key yang
  terdeteksi dari output init disimpan otomatis sebagai credential.
- LlamaStash berjalan sebagai user service (`jenderalrouter`), binary di
  `~/.local/bin`, data model di `/var/lib/jenderalrouter`.

## 7. Upgrade (manual)

1. Backup DB (otomatis/manual).
2. Ganti binary (`install -m 0755 jenderalrouter-baru /usr/local/bin/`) atau
   `docker compose up -d --build`.
3. `systemctl restart jenderalrouter`. Migrasi skema berjalan otomatis saat
   start (tabel `schema_migrations`).

## 8. Akses dari jaringan lokal (LAN)

Secara default aplikasi hanya listen di `127.0.0.1:20130` (NFR: hanya proxy
yang menghadap jaringan). Untuk akses dari PC lain di LAN, pilih salah satu:

**Opsi A — publikasikan port aplikasi (cepat):**
```bash
sudo sed -i 's|^JR_ADDR=.*|JR_ADDR=0.0.0.0:20130|' /etc/default/jenderalrouter
sudo systemctl restart jenderalrouter
sudo ufw allow from 172.26.0.0/16 to any port 20130 proto tcp  # sesuaikan subnet
```
Akses: `http://<ip-server>:20130`. Perhatian: HTTP tanpa enkripsi di LAN.

**Opsi B — Caddy di port 80 (disarankan, sesuai arsitektur PRD):**
```bash
sudo apt -y install caddy
# /etc/caddy/Caddyfile:
#   :80 {
#       reverse_proxy 127.0.0.1:20130 { flush_interval -1 }
#   }
sudo systemctl reload caddy
```
Akses: `http://<ip-server>/` — aplikasi tetap loopback, port 20130 tertutup
dari luar. Saat sudah punya domain, ganti `:80` menjadi domain agar HTTPS
otomatis aktif.

Login dari PC lain tetap normal — cookie sesi tidak beratribut `Secure`
selama `JR_PUBLIC_URL` tidak di-set https.

## 8. Batas MVP yang perlu diketahui

- SQLite tunggal (WAL) — cukup untuk ratusan request/detik pada VPS 2 vCPU;
  PostgreSQL menyusul di v1.1 bila butuh HA.
- Redis belum dipakai (`JR_REDIS_URL` dicatat, belum aktif) — cache memori
  cukup untuk satu instance.
- Endpoint `/v1/responses`, `/v1/embeddings`, gambar/audio: roadmap v1.1–v1.2.
