package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// metricsSimple koleksi metrik format teks Prometheus 0.0.4 (NFR-13).
// Implementasi ringan: counter + gauge + histogram tetap.
type metricsSimple struct {
	mu             sync.Mutex
	requests       map[string]int64 // provider|status → count
	tokens         map[string]int64 // direction → count
	upstreamErrors map[string]int64 // provider|code → count
	circuitOpen    map[int64]int    // providerID → 0/1
	durations      []float64        // detik (untuk histogram)
	sessions       int64
}

var metrics = &metricsSimple{
	requests:       map[string]int64{},
	tokens:         map[string]int64{},
	upstreamErrors: map[string]int64{},
	circuitOpen:    map[int64]int{},
}

// metricsBuckets bucket histogram latensi (detik).
var metricsBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

func (m *metricsSimple) observeRequest(provider string, status int, durationSec float64, tokensIn, tokensOut int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[fmt.Sprintf("%s|%d", provider, status)]++
	m.durations = append(m.durations, durationSec)
	if len(m.durations) > 100_000 {
		m.durations = m.durations[len(m.durations)-50_000:]
	}
	m.tokens["in"] += int64(tokensIn)
	m.tokens["out"] += int64(tokensOut)
}

func (m *metricsSimple) observeUpstreamError(provider, code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upstreamErrors[provider+"|"+code]++
}

func (m *metricsSimple) setCircuit(providerID int64, open bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := 0
	if open {
		v = 1
	}
	m.circuitOpen[providerID] = v
}

func (m *metricsSimple) setSessions(n int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = n
}

func escapeLabel(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// render menghasilkan body teks /metrics.
func (m *metricsSimple) render() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder

	b.WriteString("# HELP jr_requests_total Total request inferensi.\n")
	b.WriteString("# TYPE jr_requests_total counter\n")
	keys := make([]string, 0, len(m.requests))
	for k := range m.requests {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts := strings.SplitN(k, "|", 2)
		fmt.Fprintf(&b, "jr_requests_total{provider=%q,status=%q} %d\n", escapeLabel(parts[0]), parts[1], m.requests[k])
	}

	b.WriteString("# HELP jr_tokens_total Total token diproses.\n")
	b.WriteString("# TYPE jr_tokens_total counter\n")
	for _, dir := range []string{"in", "out"} {
		fmt.Fprintf(&b, "jr_tokens_total{direction=%q} %d\n", dir, m.tokens[dir])
	}

	b.WriteString("# HELP jr_upstream_errors_total Error upstream per provider/kode.\n")
	b.WriteString("# TYPE jr_upstream_errors_total counter\n")
	ekeys := make([]string, 0, len(m.upstreamErrors))
	for k := range m.upstreamErrors {
		ekeys = append(ekeys, k)
	}
	sort.Strings(ekeys)
	for _, k := range ekeys {
		parts := strings.SplitN(k, "|", 2)
		fmt.Fprintf(&b, "jr_upstream_errors_total{provider=%q,code=%q} %d\n", escapeLabel(parts[0]), escapeLabel(parts[1]), m.upstreamErrors[k])
	}

	b.WriteString("# HELP jr_request_duration_seconds Latensi request inferensi.\n")
	b.WriteString("# TYPE jr_request_duration_seconds histogram\n")
	counts := make([]int64, len(metricsBuckets))
	sum := 0.0
	total := int64(len(m.durations))
	for _, d := range m.durations {
		sum += d
		for i, ub := range metricsBuckets {
			if d <= ub {
				counts[i]++
			}
		}
	}
	var cum int64
	for i, ub := range metricsBuckets {
		cum = counts[i]
		fmt.Fprintf(&b, "jr_request_duration_seconds_bucket{le=%g} %d\n", ub, cum)
	}
	fmt.Fprintf(&b, "jr_request_duration_seconds_bucket{le=+Inf} %d\n", total)
	if total > 0 {
		fmt.Fprintf(&b, "jr_request_duration_seconds_sum %g\n", sum)
		fmt.Fprintf(&b, "jr_request_duration_seconds_count %d\n", total)
	}

	b.WriteString("# HELP jr_circuit_state Status circuit breaker provider (1=open).\n")
	b.WriteString("# TYPE jr_circuit_state gauge\n")
	for pid, open := range m.circuitOpen {
		fmt.Fprintf(&b, "jr_circuit_state{provider_id=\"%d\"} %d\n", pid, open)
	}

	fmt.Fprintf(&b, "# HELP jr_sessions_active Sesi dashboard aktif.\n# TYPE jr_sessions_active gauge\njr_sessions_active %d\n", m.sessions)
	return b.String()
}

func (a *App) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(metrics.render()))
}

var _ = time.Now
