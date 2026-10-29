package provider

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIsPublicIP(t *testing.T) {
	for ip, want := range map[string]bool{
		"8.8.8.8": true, "1.1.1.1": true, "2606:4700::1111": true,
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false,
		"169.254.169.254": false, "100.100.100.200": false, "100.64.0.1": false,
		"198.18.0.1": false, "0.0.0.0": false, "::1": false, "fd00::1": false, "fe80::1": false,
		"64:ff9b::a9fe:a9fe": false, "::ffff:127.0.0.1": false, "224.0.0.1": false,
	} {
		if got := IsPublicIP(net.ParseIP(ip)); got != want {
			t.Errorf("IsPublicIP(%s) = %v, mau %v", ip, got, want)
		}
	}
}

// klien publik menolak loopback saat dial — termasuk lewat redirect.
func TestPublicClientBlocksInternal(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("rahasia internal"))
	}))
	defer internal.Close()
	c := NewPublicHTTPClient(5 * time.Second)
	_, err := c.Get(internal.URL)
	if err == nil || !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("akses loopback harus diblokir, err = %v", err)
	}
}
