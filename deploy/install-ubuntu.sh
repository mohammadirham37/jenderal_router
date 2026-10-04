#!/usr/bin/env bash
# install-ubuntu.sh — instalasi JenderalRouter di VPS Ubuntu 22.04/24.04/26.04.
# Dua mode:
#   sudo ./install-ubuntu.sh docker   → Docker Compose + Caddy (HTTPS otomatis)
#   sudo ./install-ubuntu.sh binary   → binary Go + systemd (tanpa Docker)
# Target: tujuan instalasi ≤ 10 menit dari VPS kosong (metrik PRD §2).
set -euo pipefail

VERSION="${JR_VERSION:-v1.0.0}"
INSTALL_MODE="${1:-binary}"
DOWNLOAD_BASE="${JR_DOWNLOAD_BASE:-https://github.com/jenderal/jenderalrouter/releases/download/${VERSION}}"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) GOARCH="amd64" ;;
  aarch64) GOARCH="arm64" ;;
  *) echo "Arsitektur tidak didukung: $ARCH" >&2; exit 1 ;;
esac

if [[ $EUID -ne 0 ]]; then
  echo "Jalankan sebagai root: sudo $0 $INSTALL_MODE" >&2
  exit 1
fi

# ---- cek Ubuntu 22.04/24.04/26.04 ----
if [[ -f /etc/os-release ]]; then
  . /etc/os-release
  case "${VERSION_ID:-}" in
    22.04|24.04|26.04) echo ">> Ubuntu ${VERSION_ID} terdeteksi ✓" ;;
    *) echo "!! Versi Ubuntu ${VERSION_ID:-?} tidak diuji (target: 22.04/24.04/26.04). Lanjut dengan risiko sendiri." >&2 ;;
  esac
fi

echo ">> Mode instalasi: ${INSTALL_MODE}"

if [[ "$INSTALL_MODE" == "docker" ]]; then
  # ---------- Docker ----------
  if ! command -v docker >/dev/null 2>&1; then
    echo ">> Memasang Docker Engine…"
    apt-get update -y
    apt-get install -y ca-certificates curl gnupg
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
https://download.docker.com/linux/ubuntu ${VERSION_CODENAME:-noble} stable" \
      > /etc/apt/sources.list.d/docker.list
    apt-get update -y
    apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
    systemctl enable --now docker
  fi
  APP_DIR=/opt/jenderalrouter
  mkdir -p "$APP_DIR"
  cd "$APP_DIR"
  # unduh berkas deploy bila belum ada
  if [[ ! -f docker-compose.yml ]]; then
    for f in docker-compose.yml Caddyfile .env.example jenderalrouter.service; do
      curl -fsSL "${DOWNLOAD_BASE}/deploy/${f}" -o "$f" || echo "!! gagal unduh $f (salin manual)"
    done
  fi
  [[ -f .env ]] || cp .env.example .env
  echo ">> Edit ${APP_DIR}/.env (isi JR_DOMAIN) lalu jalankan: docker compose up -d"
  echo ">> (Binary image akan di-build dari source bila registry tidak tersedia.)"
  if grep -q 'JR_DOMAIN=ai.domainanda.com' .env 2>/dev/null; then
    read -r -p "Masukkan domain publik (mis. ai.domainanda.com): " DOMAIN
    [[ -n "$DOMAIN" ]] && sed -i "s|^JR_DOMAIN=.*|JR_DOMAIN=${DOMAIN}|" .env
  fi
  if command -v docker >/dev/null 2>&1; then
    docker compose up -d --build
    echo ">> Selesai! Dashboard: https://$(grep '^JR_DOMAIN=' .env | cut -d= -f2)"
  fi
  exit 0
fi

# ---------- Binary + systemd ----------
echo ">> Membuat user sistem jenderalrouter…"
id -u jenderalrouter >/dev/null 2>&1 || useradd --system --home /var/lib/jenderalrouter --shell /usr/sbin/nologin jenderalrouter
mkdir -p /var/lib/jenderalrouter /etc/default
chown jenderalrouter:jenderalrouter /var/lib/jenderalrouter
chmod 700 /var/lib/jenderalrouter

echo ">> Mengunduh binary jenderalrouter linux/${GOARCH} ${VERSION}…"
curl -fsSL "${DOWNLOAD_BASE}/jenderalrouter-linux-${GOARCH}.tar.gz" -o /tmp/jr.tar.gz
tar -xzf /tmp/jr.tar.gz -C /tmp
install -m 0755 /tmp/jenderalrouter /usr/local/bin/jenderalrouter
rm -f /tmp/jr.tar.gz /tmp/jenderalrouter

echo ">> Memasang service systemd…"
curl -fsSL "${DOWNLOAD_BASE}/deploy/jenderalrouter.service" -o /etc/systemd/system/jenderalrouter.service \
  || echo "!! salin deploy/jenderalrouter.service manual"
# environment default
[[ -f /etc/default/jenderalrouter ]] || cat > /etc/default/jenderalrouter <<'ENV'
# Konfigurasi JenderalRouter (PRD §10.1)
JR_ADDR=127.0.0.1:20130
JR_DATA_DIR=/var/lib/jenderalrouter
JR_TZ=Asia/Jakarta
JR_LOG_PROMPTS=false
# Opsional:
# JR_PUBLIC_URL=https://ai.domainanda.com
# JR_LLAMASTASH_URL=http://127.0.0.1:11435/v1
ENV
chmod 600 /etc/default/jenderalrouter

# firewall: hanya 80/443 ke publik (port app tetap loopback)
if command -v ufw >/dev/null 2>&1; then
  echo ">> UFW: membuka 80/443 (SSH tetap aman bila sudah diizinkan)…"
  ufw allow 80/tcp >/dev/null 2>&1 || true
  ufw allow 443/tcp >/dev/null 2>&1 || true
fi

systemctl daemon-reload
systemctl enable --now jenderalrouter
sleep 2
systemctl --no-pager status jenderalrouter | head -5 || true

cat <<'NEXT'

==========================================================
 JenderalRouter terpasang!
 1. (Opsional) pasang Caddy untuk HTTPS:
      sudo apt-get install -y caddy
      # Caddyfile: ai.domainanda.com { reverse_proxy 127.0.0.1:20130 { flush_interval -1 } }
 2. Buka dashboard: http://127.0.0.1:20130 (via SSH tunnel: ssh -L 20130:127.0.0.1:20130 vps)
 3. Wizard pertama akan meminta akun admin — tidak ada password bawaan.
 4. (Opsional) pasang LlamaStash di host:
      curl -fsSL https://llamastash.dev/install.sh | sh
      llamastash init --recommended --json
    lalu di dashboard tambahkan provider "LlamaStash (lokal)".
==========================================================
NEXT
