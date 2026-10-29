package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// blockedNets rentang yang tidak boleh dihubungi tool publik (web_fetch):
// selain loopback/private/link-local bawaan net.IP, juga CGNAT/Tailscale,
// metadata cloud non-169.254, NAT64, benchmark, dokumentasi, dan reserved.
var blockedNets = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "255.255.255.255/32",
		"64:ff9b::/96", "64:ff9b:1::/48", "2001:db8::/32", "100::/64", "2002::/16",
	} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

// IsPublicIP true bila ip aman dihubungi dari server (bukan jaringan internal).
func IsPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// publicOnlyControl menolak koneksi ke IP non-publik TEPAT saat dial —
// berlaku juga untuk redirect dan DNS rebinding (resolusi ulang).
func publicOnlyControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if !IsPublicIP(net.ParseIP(host)) {
		return fmt.Errorf("%w: %s", ErrSSRFBlocked, host)
	}
	return nil
}

// NewPublicHTTPClient klien HTTP yang hanya bisa menghubungi IP publik,
// tanpa proxy lingkungan, dengan redirect terbatas & diperiksa ulang.
func NewPublicHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: publicOnlyControl}
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           func(ctx context.Context, n, a string) (net.Conn, error) { return dialer.DialContext(ctx, n, a) },
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("terlalu banyak redirect")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("redirect ke skema %q ditolak", req.URL.Scheme)
			}
			return nil
		},
	}
}
