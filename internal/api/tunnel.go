package api

// Status Cloudflare Tunnel untuk kartu Pengaturan: mendeteksi cloudflared
// di host (binary, proses, service systemd) dan membaca ringkasan
// /etc/cloudflared/config.yml bila ada — tanpa menyentuh prosesnya.

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
)

func (a *App) handleCloudflareStatus(w http.ResponseWriter, r *http.Request) {
	bin := ""
	if p, err := exec.LookPath("cloudflared"); err == nil {
		bin = p
	}
	running := false
	if out, err := exec.Command("pgrep", "-x", "cloudflared").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) != "" {
		running = true
	}
	svcState := ""
	if out, err := exec.Command("systemctl", "is-active", "cloudflared").CombinedOutput(); err == nil {
		svcState = strings.TrimSpace(string(out))
	}

	cfgPath := "/etc/cloudflared/config.yml"
	hostname, service := "", ""
	cfgFound := false
	if b, err := os.ReadFile(cfgPath); err == nil {
		cfgFound = true
		for _, line := range strings.Split(string(b), "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "hostname:") {
				hostname = strings.TrimSpace(strings.TrimPrefix(t, "hostname:"))
			}
			if strings.HasPrefix(t, "service:") {
				service = strings.TrimSpace(strings.TrimPrefix(t, "service:"))
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"installed":       bin != "",
		"bin":             bin,
		"running":         running,
		"service_state":   svcState,
		"config_found":    cfgFound,
		"hostname":        hostname,
		"ingress_service": service,
		"gateway_addr":    a.cfg.Addr,
	})
}
