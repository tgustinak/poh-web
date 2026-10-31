package admin

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// VPSHandler serves the VPS monitoring page.
type VPSHandler struct{}

// NewVPSHandler creates a VPS monitoring handler.
func NewVPSHandler() *VPSHandler {
	return &VPSHandler{}
}

// VPSPage renders the VPS monitoring dashboard.
func (h *VPSHandler) VPSPage(w http.ResponseWriter, r *http.Request) {
	csrf, _ := auth.CsrfFromRequest(r)

	stats := collectStats()

	admintemplates.AdminVPS(
		csrf,
		stats["uptime"],
		stats["load"],
		stats["mem_used"],
		stats["mem_total"],
		stats["mem_percent"],
		stats["disk_used"],
		stats["disk_total"],
		stats["disk_percent"],
		stats["go_version"],
		stats["goroutines"],
	).Render(r.Context(), w)
}

func collectStats() map[string]string {
	s := map[string]string{
		"go_version":  runtime.Version(),
		"goroutines":  strconv.Itoa(runtime.NumGoroutine()),
	}

	// Uptime from /proc/uptime
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		if parts := strings.Fields(string(data)); len(parts) > 0 {
			if secs, err := strconv.ParseFloat(parts[0], 64); err == nil {
				d := time.Duration(secs * float64(time.Second))
				s["uptime"] = formatDuration(d)
			}
		}
	}
	if s["uptime"] == "" {
		s["uptime"] = "n/a"
	}

	// Load average from /proc/loadavg
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		if parts := strings.Fields(string(data)); len(parts) > 0 {
			s["load"] = parts[0]
		}
	}
	if s["load"] == "" {
		s["load"] = "n/a"
	}

	// Memory from /proc/meminfo
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, available int64
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			n, err := strconv.Atoi(fields[1])
			if err != nil {
				continue
			}
			mb := int64(n) / 1024 // kB to MB
			switch {
			case strings.HasPrefix(line, "MemTotal:"):
				total = mb
			case strings.HasPrefix(line, "MemAvailable:"):
				available = mb
			}
		}
		used := total - available
		s["mem_total"] = strconv.FormatInt(total, 10)
		s["mem_used"] = strconv.FormatInt(used, 10)
		if total > 0 {
			s["mem_percent"] = strconv.FormatFloat(float64(used)/float64(total)*100, 'f', 1, 64)
		} else {
			s["mem_percent"] = "0.0"
		}
	}
	if s["mem_total"] == "" {
		s["mem_total"] = "n/a"
		s["mem_used"] = "n/a"
		s["mem_percent"] = "n/a"
	}

	// Disk from statfs on root
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		total := int64(stat.Blocks) * int64(stat.Bsize) / (1024 * 1024 * 1024)
		free := int64(stat.Bfree) * int64(stat.Bsize) / (1024 * 1024 * 1024)
		used := total - free
		s["disk_total"] = strconv.FormatInt(total, 10)
		s["disk_used"] = strconv.FormatInt(used, 10)
		if total > 0 {
			s["disk_percent"] = strconv.FormatFloat(float64(used)/float64(total)*100, 'f', 1, 64)
		} else {
			s["disk_percent"] = "0.0"
		}
	}
	if s["disk_total"] == "" {
		s["disk_total"] = "n/a"
		s["disk_used"] = "n/a"
		s["disk_percent"] = "n/a"
	}

	return s
}

func formatDuration(d time.Duration) string {
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute

	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if len(parts) == 0 {
		return "<1m"
	}
	return strings.Join(parts, " ")
}
