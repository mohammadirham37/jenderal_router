package api

// Statistik sistem live untuk dashboard: CPU, RAM, disk, GPU (bila ada).
// Sampler berjalan di latar (2 dtk) mengisi snapshot + riwayat CPU;
// handler hanya membaca snapshot — tanpa spawn proses per-request.
// Hanya Linux yang punya /proc; di OS lain sampler idle (supported=false).

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type gpuStats struct {
	Name       string   `json:"name,omitempty"`
	Percent    *float64 `json:"percent"`
	MemUsedGB  *float64 `json:"mem_used_gb"`
	MemTotalGB *float64 `json:"mem_total_gb"`
}

type systemStats struct {
	Supported   bool      `json:"supported"`
	CPUPercent  float64   `json:"cpu_percent"`
	CPUCores    int       `json:"cpu_cores"`
	CPUHistory  []float64 `json:"cpu_history"`
	RAMUsedGB   float64   `json:"ram_used_gb"`
	RAMTotalGB  float64   `json:"ram_total_gb"`
	DiskUsedGB  float64   `json:"disk_used_gb"`
	DiskTotalGB float64   `json:"disk_total_gb"`
	GPU         *gpuStats `json:"gpu"`
	UpdatedAt   string    `json:"updated_at"`
}

var (
	sysMu         sync.Mutex
	sysCurrent    systemStats
	prevTotalTick uint64
	prevIdleTick  uint64
	hasPrevTick   bool
	sysGPUSkip    int // hitungan untuk menjeda query GPU
)

func cpuCoresCount() int {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return runtime.NumCPU()
	}
	n := strings.Count(string(b), "processor")
	if n == 0 {
		n = runtime.NumCPU()
	}
	return n
}

// parseProcStat ambil baris "cpu " agregat → total & idle tick.
func parseProcStat(line string) (total, idle uint64, ok bool) {
	f := strings.Fields(line)
	if len(f) < 5 || f[0] != "cpu" {
		return 0, 0, false
	}
	var nums []uint64
	for _, s := range f[1:] {
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		nums = append(nums, v)
	}
	for _, v := range nums {
		total += v
	}
	// idle = idle + iowait
	idle = nums[3]
	if len(nums) > 4 {
		idle += nums[4]
	}
	return total, idle, true
}

// parseMemInfo → total & used byte (used = total - available).
func parseMemInfo(body string) (total, used uint64, ok bool) {
	var avail uint64
	haveTotal, haveAvail := false, false
	for _, line := range strings.Split(body, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total, haveTotal = v*1024, true
		case strings.HasPrefix(line, "MemAvailable:"):
			avail, haveAvail = v*1024, true
		}
	}
	if !haveTotal || !haveAvail {
		return 0, 0, false
	}
	return total, total - avail, true
}

func queryGPUSMI() *gpuStats {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=name,utilization.gpu,memory.used,memory.total",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	parts := strings.Split(line, ",")
	if len(parts) < 4 {
		return nil
	}
	pct, err1 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	mu, err2 := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	mt, err3 := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return nil
	}
	return &gpuStats{
		Name:       strings.TrimSpace(parts[0]),
		Percent:    &pct,
		MemUsedGB:  fp(round2(mu / 1024)),
		MemTotalGB: fp(round2(mt / 1024)),
	}
}

func fp(f float64) *float64 { return &f }

// statsWorker sampel sistem tiap 2 detik sampai ctx selesai.
func (a *App) statsWorker(ctx context.Context) {
	if runtime.GOOS != "linux" {
		sysMu.Lock()
		sysCurrent.Supported = false
		sysMu.Unlock()
		return
	}
	sysMu.Lock()
	sysCurrent.Supported = true
	sysCurrent.CPUCores = cpuCoresCount()
	sysMu.Unlock()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// CPU
			if b, err := os.ReadFile("/proc/stat"); err == nil {
				line := strings.SplitN(string(b), "\n", 2)[0]
				if total, idle, ok := parseProcStat(line); ok {
					sysMu.Lock()
					if hasPrevTick && total > prevTotalTick {
						dt := total - prevTotalTick
						di := idle - prevIdleTick
						sysCurrent.CPUPercent = round2((1 - float64(di)/float64(dt)) * 100)
					}
					prevTotalTick, prevIdleTick, hasPrevTick = total, idle, true
					sysCurrent.CPUHistory = append(sysCurrent.CPUHistory, sysCurrent.CPUPercent)
					if len(sysCurrent.CPUHistory) > 60 {
						sysCurrent.CPUHistory = sysCurrent.CPUHistory[1:]
					}
					sysMu.Unlock()
				}
			}
			// RAM
			if b, err := os.ReadFile("/proc/meminfo"); err == nil {
				if total, used, ok := parseMemInfo(string(b)); ok {
					sysMu.Lock()
					sysCurrent.RAMTotalGB = round2(float64(total) / 1e9)
					sysCurrent.RAMUsedGB = round2(float64(used) / 1e9)
					sysMu.Unlock()
				}
			}
			// Disk (data dir)
			var st syscall.Statfs_t
			if err := syscall.Statfs(a.cfg.DataDir, &st); err == nil {
				total := float64(st.Blocks) * float64(st.Bsize)
				free := float64(st.Bavail) * float64(st.Bsize)
				sysMu.Lock()
				sysCurrent.DiskTotalGB = round2(total / 1e9)
				sysCurrent.DiskUsedGB = round2((total - free) / 1e9)
				sysMu.Unlock()
			}
			// GPU — query tiap tick ke-3 (~6 dtk)
			sysMu.Lock()
			sysGPUSkip = (sysGPUSkip + 1) % 3
			doGPU := sysGPUSkip == 0 || sysCurrent.GPU == nil && sysGPUSkip == 1
			sysMu.Unlock()
			if doGPU {
				if g := queryGPUSMI(); g != nil {
					sysMu.Lock()
					sysCurrent.GPU = g
					sysMu.Unlock()
				}
			}
			sysMu.Lock()
			sysCurrent.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			sysMu.Unlock()
		}
	}
}

func (a *App) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	sysMu.Lock()
	snap := sysCurrent
	if snap.CPUHistory != nil {
		snap.CPUHistory = append([]float64(nil), snap.CPUHistory...)
	} else {
		snap.CPUHistory = []float64{}
	}
	sysMu.Unlock()
	writeJSON(w, http.StatusOK, snap)
}
