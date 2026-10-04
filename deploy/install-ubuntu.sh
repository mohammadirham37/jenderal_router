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
  BUILD_VER="$(git -C "$REPO_DIR" describe --tags --always --dirty 2>/dev/null || echo dev)"
  ( cd "$REPO_DIR" \
    && CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -trimpath \
       -ldflags "-s -w -X github.com/jenderal/jenderalrouter/internal/api.Version=${BUILD_VER}" \
       -o /tmp/jenderalrouter-bin ./cmd/jenderalrouter )
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

    # ---- helper pembaruan dari dashboard (opt-in) ----
    # Service (user jenderalrouter) bisa git pull + build sendiri; yang butuh
    # root hanya memasang binary hasil build + restart service — diberikan
    # lewat helper root yang SEMPIT dengan sudoers NOPASSWD terbatas.
    read -r -p "Aktifkan tombol 'Perbarui' dari dashboard (sudo terbatas untuk helper ini)? [Y/n]: " ANS_UPDATE || ANS_UPDATE="Y"
    ANS_UPDATE="${ANS_UPDATE:-Y}"
    if [[ ! "$ANS_UPDATE" =~ ^[Nn] ]]; then
      install -d -m 0755 /usr/local/lib/jenderalrouter
      cat > /usr/local/lib/jenderalrouter/apply-update.sh <<'APPLY'
#!/bin/bash
# Helper pembaruan JenderalRouter — dipanggil service via sudo NOPASSWD.
# Sengaja sempit: hanya memasang binary hasil build service (yang sudah
# lolos smoke test) dan me-restart service ini.
# CATATAN RISIKO: binary distage oleh user service lalu dieksekusi root.
# Bila tidak menerimanya, hapus /etc/sudoers.d/jenderalrouter-update.
set -euo pipefail
STAGE=/var/lib/jenderalrouter/updates/jenderalrouter.new
case "${1:-}" in
  check)
    exit 0
    ;;
  install)
    [ -f "$STAGE" ]
    /usr/bin/systemctl stop jenderalrouter || true
    /usr/bin/install -m 0755 "$STAGE" /usr/local/bin/jenderalrouter
    rm -f "$STAGE"
    /usr/bin/systemctl start jenderalrouter
    ;;
  *)
    exit 2
    ;;
esac
APPLY
      chown root:root /usr/local/lib/jenderalrouter/apply-update.sh
      chmod 0755 /usr/local/lib/jenderalrouter/apply-update.sh
      {
        echo "jenderalrouter ALL=(root) NOPASSWD: /usr/local/lib/jenderalrouter/apply-update.sh check"
        echo "jenderalrouter ALL=(root) NOPASSWD: /usr/local/lib/jenderalrouter/apply-update.sh install"
      } > /etc/sudoers.d/jenderalrouter-update
      if visudo -cf /etc/sudoers.d/jenderalrouter-update >/dev/null; then
        chmod 0440 /etc/sudoers.d/jenderalrouter-update
        echo ">> Helper pembaruan dashboard aktif (sudoers tervalidasi)."
      else
        echo "!! sudoers tidak valid — helper tidak diaktifkan." >&2
        rm -f /etc/sudoers.d/jenderalrouter-update
      fi
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

    # compose dijalankan LANGSUNG dari deploy/ di repo — build context `..`
    # menunjuk root repo (berisi source), jadi tanpa penyalinan ke /opt.
    cd "$REPO_DIR/deploy"

    if [[ ! -f .env ]]; then
      cp .env.example .env
    fi
    read -r -p "Masukkan domain publik (mis. ai.domainanda.com; kosongkan bila belum punya domain): " DOMAIN || DOMAIN=""
    if [[ -n "$DOMAIN" ]]; then
      sed -i "s|^JR_DOMAIN=.*|JR_DOMAIN=${DOMAIN}|" .env
      if grep -q '^JR_PUBLIC_URL=' .env; then
        sed -i "s|^JR_PUBLIC_URL=.*|JR_PUBLIC_URL=https://${DOMAIN}|" .env
      else
        echo "JR_PUBLIC_URL=https://${DOMAIN}" >> .env
      fi
    else
      sed -i "s|^JR_DOMAIN=.*|JR_DOMAIN=localhost|" .env
      # tanpa domain: PublicURL kosong agar cookie tidak Secure (akses via
      # tunnel http://localhost:20130), dan bersihkan nilai lama yang salah
      sed -i "s|^JR_PUBLIC_URL=.*|JR_PUBLIC_URL=|" .env
      sed -i "/^#\?JR_PUBLIC_URL=https:\/\/localhost/d" .env
      echo "!! Tanpa domain: Caddy tidak akan terbit sertifikat — akses lewat SSH tunnel ke 20130."
    fi

    echo ">> Menjalankan docker compose (build image dari source — beberapa menit pertama)…"
    docker compose up -d --build

    setup_firewall

    DASH_DOMAIN="$(grep '^JR_DOMAIN=' .env | cut -d= -f2)"
    cat <<NEXT

==========================================================
 JenderalRouter (Docker Compose) terpasang!
 Perintah : cd ${REPO_DIR}/deploy && docker compose logs -f
 Dashboard: https://${DASH_DOMAIN}
            (bila domain belum diarahkan, akses via SSH tunnel:
             ssh -p PORT_SSH_ANDA -L 20130:127.0.0.1:20130 user@server
             lalu buka http://localhost:20130)
 Cek      : curl http://127.0.0.1:20130/healthz
 Wizard pertama akan meminta akun admin (tanpa password bawaan).
==========================================================
NEXT
    ;;

  *)
    echo "Mode tidak dikenal: $INSTALL_MODE (pakai: binary | docker)" >&2
    exit 1
    ;;
esac
