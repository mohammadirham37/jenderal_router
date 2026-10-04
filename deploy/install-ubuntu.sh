#!/usr/bin/env bash
# install-ubuntu.sh — instalasi JenderalRouter di VPS Ubuntu 22.04/24.04/26.04.
#
# Pakai (dari hasil `git clone`):
#   sudo ./deploy/install-ubuntu.sh binary   → binary Go + systemd (tanpa Docker)
#   sudo ./deploy/install-ubuntu.sh docker   → Docker Compose + Caddy (HTTPS otomatis)
#
# Semua berkas (compose, Caddyfile, unit systemd) diambil dari repo lokal —
# tanpa unduhan. Binary mode: coba unduh rilis GitHub; bila belum ada rilis,
# otomatis build dari source (memasang Go bila perlu).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
APP_DIR=/opt/jenderalrouter
GOARCH="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
REPO_RAW="https://github.com/mohammadirham37/jenderal_router"
RELEASE="${JR_RELEASE:-latest}"   # tag rilis, mis. v1.0.0

INSTALL_MODE="${1:-binary}"

if [[ $EUID -ne 0 ]]; then
  echo "Jalankan sebagai root: sudo $0 $INSTALL_MODE" >&2
  exit 1
fi

# ---- cek Ubuntu (CATATAN: jangan menamai variabel kita VERSION/NAME —
#      `. /etc/os-release` menimpanya) ----
if [[ -f /etc/os-release ]]; then
  . /etc/os-release
  case "${VERSION_ID:-}" in
    22.04|24.04|26.04) echo ">> Ubuntu ${VERSION_ID} terdeteksi ✓" ;;
    *) echo "!! Versi Ubuntu ${VERSION_ID:-?} tidak diuji (target 22.04/24.04/26.04); lanjut dengan risiko sendiri." >&2 ;;
  esac
fi
echo ">> Mode instalasi: ${INSTALL_MODE}"
echo ">> Repo lokal: ${REPO_DIR}"

# ---- firewall: izinkan port SSH AKTUAL (bisa saja bukan 22) lalu 80/443 ----
setup_firewall() {
  command -v ufw >/dev/null 2>&1 || { echo ">> UFW tidak tersedia; lewati konfigurasi firewall."; return; }
  local ports=()
  # port dari proses sshd yang sedang listening
  if command -v ss >/dev/null 2>&1; then
    while IFS= read -r p; do [[ -n "$p" ]] && ports+=("$p"); done < <(
      ss -tlnp 2>/dev/null | awk '/sshd|ssh/ {print $4}' | grep -oE '[0-9]+$' | sort -u)
  fi
  # fallback: konfigurasi sshd
  local cfg
  cfg="$(grep -rhsE '^[[:space:]]*Port[[:space:]]+[0-9]+' /etc/ssh/sshd_config /etc/ssh/sshd_config.d/ 2>/dev/null | awk '{print $2}' | sort -u || true)"
  while IFS= read -r p; do [[ -n "$p" ]] && ports+=("$p"); done <<< "$cfg"
  ports+=(22)
  local seen=" "
  for p in "${ports[@]}"; do
    [[ "$seen" == *" $p "* ]] && continue
    seen="$seen$p "
    ufw allow "$p/tcp" >/dev/null 2>&1 && echo ">> UFW: izinkan SSH $p/tcp" || true
  done
  ufw allow 80/tcp  >/dev/null 2>&1 || true
  ufw allow 443/tcp >/dev/null 2>&1 || true
  if ufw status 2>/dev/null | grep -q "Status: active"; then
    echo ">> UFW sudah aktif; aturan diperbarui (SSH tidak terputus)."
  else
    ufw --force enable >/dev/null 2>&1 && echo ">> UFW diaktifkan (80/443 publik; SSH $seen tetap terbuka)." || true
  fi
}

# ---- mode BINARY: rilis GitHub → fallback build dari source ----
install_binary() {
  local url
  if [[ "$RELEASE" == "latest" ]]; then
    url="$REPO_RAW/releases/latest/download/jenderalrouter-linux-${GOARCH}.tar.gz"
  else
    url="$REPO_RAW/releases/download/${RELEASE}/jenderalrouter-linux-${GOARCH}.tar.gz"
  fi
  echo ">> Mencoba unduh binary rilis ${RELEASE} (linux/${GOARCH})…"
  if curl -fsSL --max-time 60 "$url" -o /tmp/jr.tar.gz 2>/dev/null; then
    tar -xzf /tmp/jr.tar.gz -C /tmp
    install -m 0755 "/tmp/jenderalrouter-linux-${GOARCH}" /usr/local/bin/jenderalrouter
    rm -f /tmp/jr.tar.gz; rm -f /tmp/jenderalrouter-linux-*
    return 0
  fi
  echo ">> Rilis belum tersedia (404) — build dari source di server…"
  if ! command -v go >/dev/null 2>&1; then
    local gov
    gov="$(curl -fsSL --max-time 20 'https://go.dev/dl/?mode=json' \
      | grep -o '"version": *"go[0-9.]*"' | head -1 | grep -oE '[0-9]+\.[0-9.]+' || true)"
    gov="${gov:-1.27.1}"
    echo ">> Memasang Go ${gov} ke /usr/local/go…"
    curl -fsSL --max-time 300 "https://go.dev/dl/go${gov}.linux-${GOARCH}.tar.gz" -o /tmp/go.tgz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tgz
    rm -f /tmp/go.tgz
    export PATH="/usr/local/go/bin:$PATH"
  fi
  echo ">> Build binary (linux/${GOARCH})…"
  ( cd "$REPO_DIR" \
    && CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -trimpath \
       -ldflags "-s -w" -o /tmp/jenderalrouter-bin ./cmd/jenderalrouter )
  install -m 0755 /tmp/jenderalrouter-bin /usr/local/bin/jenderalrouter
  rm -f /tmp/jenderalrouter-bin
}

case "$INSTALL_MODE" in
  binary)
    echo ">> Membuat user sistem jenderalrouter…"
    id -u jenderalrouter >/dev/null 2>&1 || \
      useradd --system --home /var/lib/jenderalrouter --shell /usr/sbin/nologin jenderalrouter
    mkdir -p /var/lib/jenderalrouter
    chown jenderalrouter:jenderalrouter /var/lib/jenderalrouter
    chmod 700 /var/lib/jenderalrouter

    install_binary

    echo ">> Memasang service systemd (dari repo lokal)…"
    install -m 0644 "$SCRIPT_DIR/jenderalrouter.service" /etc/systemd/system/jenderalrouter.service

    if [[ ! -f /etc/default/jenderalrouter ]]; then
      cat > /etc/default/jenderalrouter <<'ENV'
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
    fi

    setup_firewall

    systemctl daemon-reload
    systemctl enable --now jenderalrouter
    sleep 2
    systemctl --no-pager --lines=0 status jenderalrouter || true

    cat <<'NEXT'

==========================================================
 JenderalRouter (binary + systemd) terpasang!
 Status : systemctl status jenderalrouter
 Log    : journalctl -u jenderalrouter -f
 Dashboard: http://127.0.0.1:20130
   (remote: ssh -p PORT_SSH_ANDA -L 20130:127.0.0.1:20130 user@server)
 HTTPS  : pasang Caddy lalu buat /etc/caddy/Caddyfile:
   domain-anda.com {
       reverse_proxy 127.0.0.1:20130 { flush_interval -1 }
   }
 Wizard pertama akan meminta akun admin (tanpa password bawaan).
==========================================================
NEXT
    ;;

  docker)
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

    echo ">> Menyalin berkas deploy dari repo lokal ke ${APP_DIR}…"
    mkdir -p "$APP_DIR"
    install -m 0644 "$SCRIPT_DIR/docker-compose.yml" "$APP_DIR/docker-compose.yml"
    install -m 0644 "$SCRIPT_DIR/Caddyfile"          "$APP_DIR/Caddyfile"
    install -m 0644 "$SCRIPT_DIR/.env.example"       "$APP_DIR/.env.example"
    cd "$APP_DIR"

    if [[ ! -f .env ]]; then
      cp .env.example .env
    fi
    # isi domain bila masih contoh
    if grep -q '^JR_DOMAIN=ai.domainanda.com' .env || ! grep -q '^JR_DOMAIN=' .env; then
      read -r -p "Masukkan domain publik (mis. ai.domainanda.com; kosongkan bila belum punya domain): " DOMAIN || DOMAIN=""
      if [[ -n "$DOMAIN" ]]; then
        sed -i "s|^JR_DOMAIN=.*|JR_DOMAIN=${DOMAIN}|" .env
      else
        # tanpa domain: Caddy dilewati, akses via SSH tunnel ke 20130
        sed -i "s|^JR_DOMAIN=.*|JR_DOMAIN=localhost|" .env
        echo "!! Tanpa domain, Caddy akan gagal terbit sertifikat — akses lewat SSH tunnel ke port 20130."
      fi
    fi

    echo ">> Menjalankan docker compose (build image dari source — beberapa menit pertama)…"
    docker compose up -d --build

    setup_firewall

    cat <<NEXT

==========================================================
 JenderalRouter (Docker Compose) terpasang!
 Perintah : cd ${APP_DIR} && docker compose logs -f
 Dashboard: https://$(grep '^JR_DOMAIN=' ${APP_DIR}/.env | cut -d= -f2)
            (bila domain belum diarahkan, akses via SSH tunnel:
             ssh -p PORT_SSH_ANDA -L 20130:127.0.0.1:20130 user@server
             lalu buka http://localhost:20130)
 Wizard pertama akan meminta akun admin (tanpa password bawaan).
==========================================================
NEXT
    ;;

  *)
    echo "Mode tidak dikenal: $INSTALL_MODE (pakai: binary | docker)" >&2
    exit 1
    ;;
esac
