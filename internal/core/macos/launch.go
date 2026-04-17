package macos

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// LaunchTestResult summarizes what happened when we briefly launched a
// cloned app to see whether it stays alive.
type LaunchTestResult struct {
	Attempted       bool
	Launched        bool   // the `open` call itself succeeded
	Survived        bool   // app was still alive at the end of the timeout
	SurvivedMs      int64  // ms between first-sighting and last-check
	CrashSummary    string // e.g. "EXC_BREAKPOINT (SIGTRAP) at ares_dns_rr_get_ttl"
	CrashReportPath string // absolute path to the .ips in DiagnosticReports
	Note            string // free-form, shown when Survived is false without a crash report
}

// LaunchTest opens appPath with /usr/bin/open, waits up to `timeout` for the
// process to appear and stay alive, and cleans up on success. If the
// process exits early, tries to correlate with a .ips crash report.
//
// NOTE: launch tests actually run the cloned app, which means it will start
// creating its own sandbox container, network connections, etc. Callers
// should only run this when the user has explicitly opted in.
func LaunchTest(ctx context.Context, ex Execer, appPath, bundleID string, timeout time.Duration) *LaunchTestResult {
	r := &LaunchTestResult{Attempted: true}
	if timeout <= 0 {
		// Electron/Chromium apps routinely take 3-5s to bootstrap. Default
		// needs to comfortably exceed that or we get false negatives.
		timeout = 10 * time.Second
	}

	home := os.Getenv("HOME")
	crashDir := ""
	var before crashSnapshot
	if home != "" {
		crashDir = filepath.Join(home, "Library", "Logs", "DiagnosticReports")
		before = snapshotCrashDir(crashDir)
	}

	// -g = don't bring to front; -n = new instance (don't reuse running one).
	_, stderr, code, err := ex.Run(ctx, "open", "-g", "-n", appPath)
	if err != nil {
		r.Note = fmt.Sprintf("open failed: %v", err)
		return r
	}
	if code != 0 {
		r.Note = fmt.Sprintf("open returned exit=%d: %s", code, strings.TrimSpace(string(stderr)))
		return r
	}
	r.Launched = true

	start := time.Now()
	var pid int
	// Poll throughout the full timeout window for the process to appear.
	// Electron/Chromium apps can take 3-5s before the main process is
	// visible in ps. We scan `ps -axo pid,command` for the target exe path
	// which is what Activity Monitor uses and reliably finds GUI processes.
	exePath := filepath.Join(appPath, "Contents", "MacOS") + "/"
	discoveryDeadline := start.Add(timeout)
	for time.Now().Before(discoveryDeadline) {
		time.Sleep(300 * time.Millisecond)
		if pid = findPIDByExePath(ctx, ex, exePath); pid > 0 {
			break
		}
		if pid = findPIDByBundleID(ctx, ex, bundleID); pid > 0 {
			break
		}
	}
	if pid <= 0 {
		r.Survived = false
		r.Note = "process never became visible — likely crashed on startup"
		if crashDir != "" {
			r.CrashSummary, r.CrashReportPath = findNewCrashReport(crashDir, before, bundleID, appPath)
		}
		return r
	}

	// Watch for early exit. Give at least 3s past discovery so Electron
	// helpers finishing their init doesn't get mistaken for main exit.
	watchDeadline := time.Now().Add(3 * time.Second)
	if orig := start.Add(timeout); orig.After(watchDeadline) {
		watchDeadline = orig
	}
	for time.Now().Before(watchDeadline) {
		time.Sleep(300 * time.Millisecond)
		if !processAlive(pid) {
			// Try to find replacement process (main may have respawned
			// under a different pid, common in Electron multi-process).
			exePath := filepath.Join(appPath, "Contents", "MacOS") + "/"
			if newPID := findPIDByExePath(ctx, ex, exePath); newPID > 0 && newPID != pid {
				pid = newPID
				continue
			}
			r.Survived = false
			r.SurvivedMs = time.Since(start).Milliseconds()
			if crashDir != "" {
				r.CrashSummary, r.CrashReportPath = findNewCrashReport(crashDir, before, bundleID, appPath)
			}
			if r.CrashSummary == "" {
				r.Note = "process exited before timeout (no crash report found)"
			}
			return r
		}
	}

	// Still alive — kill it cleanly.
	r.Survived = true
	r.SurvivedMs = time.Since(start).Milliseconds()
	_ = syscall.Kill(pid, syscall.SIGTERM)
	time.Sleep(400 * time.Millisecond)
	if processAlive(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	return r
}

var lsappinfoPIDRE = regexp.MustCompile(`"pid"\s*=\s*(\d+)`)

// findPIDByExePath scans `ps -axo pid,command` for the first process whose
// command contains exePathPrefix. Most reliable on macOS for GUI apps —
// pgrep -f sometimes misses LaunchServices-launched processes.
func findPIDByExePath(ctx context.Context, ex Execer, exePathPrefix string) int {
	stdout, _, _, _ := ex.Run(ctx, "ps", "-axo", "pid=,command=")
	for _, line := range strings.Split(string(stdout), "\n") {
		if !strings.Contains(line, exePathPrefix) {
			continue
		}
		trimmed := strings.TrimLeft(line, " ")
		var pid int
		if _, err := fmt.Sscanf(trimmed, "%d", &pid); err == nil && pid > 0 {
			return pid
		}
	}
	return 0
}

func findPIDByBundleID(ctx context.Context, ex Execer, bundleID string) int {
	// lsappinfo info -only pid <bundleID> → e.g.  "pid"=31234
	stdout, _, _, _ := ex.Run(ctx, "lsappinfo", "info", "-only", "pid", bundleID)
	m := lsappinfoPIDRE.FindStringSubmatch(string(stdout))
	if len(m) != 2 {
		return 0
	}
	var pid int
	if _, err := fmt.Sscanf(m[1], "%d", &pid); err == nil && pid > 0 {
		return pid
	}
	return 0
}

func processAlive(pid int) bool {
	// signal 0 checks process existence without affecting it.
	return syscall.Kill(pid, 0) == nil
}

// ——— Crash report correlation ——————————————————————————————————————————

type crashSnapshot struct {
	files map[string]time.Time
}

func snapshotCrashDir(dir string) crashSnapshot {
	s := crashSnapshot{files: map[string]time.Time{}}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		s.files[e.Name()] = info.ModTime()
	}
	return s
}

// findNewCrashReport scans dir for .ips files newer than the snapshot and
// returns the first one whose header bundleID / app_name matches either
// bundleID or the leaf of appPath. Returns a short human-readable summary.
func findNewCrashReport(dir string, before crashSnapshot, bundleID, appPath string) (summary, path string) {
	appName := strings.TrimSuffix(filepath.Base(appPath), ".app")
	entries, _ := os.ReadDir(dir)
	type candidate struct {
		path string
		mt   time.Time
	}
	var fresh []candidate
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".ips") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if prev, existed := before.files[e.Name()]; existed && !info.ModTime().After(prev) {
			continue
		}
		fresh = append(fresh, candidate{path: filepath.Join(dir, e.Name()), mt: info.ModTime()})
	}
	// newest first
	for i := 0; i < len(fresh); i++ {
		for j := i + 1; j < len(fresh); j++ {
			if fresh[j].mt.After(fresh[i].mt) {
				fresh[i], fresh[j] = fresh[j], fresh[i]
			}
		}
	}

	for _, c := range fresh {
		data, err := os.ReadFile(c.path)
		if err != nil {
			continue
		}
		idx := strings.Index(string(data), "\n")
		if idx < 0 {
			continue
		}
		headerLine := string(data[:idx])
		body := data[idx+1:]

		var header struct {
			BundleID string `json:"bundleID"`
			AppName  string `json:"app_name"`
			ProcPath string `json:"procPath"`
		}
		if err := json.Unmarshal([]byte(headerLine), &header); err != nil {
			continue
		}
		if header.BundleID != bundleID && header.AppName != appName && !strings.HasPrefix(header.ProcPath, appPath) {
			continue
		}

		var b struct {
			Exception struct {
				Type   string `json:"type"`
				Signal string `json:"signal"`
			} `json:"exception"`
			Threads []struct {
				Triggered bool `json:"triggered"`
				Frames    []struct {
					Symbol string `json:"symbol"`
				} `json:"frames"`
			} `json:"threads"`
		}
		_ = json.Unmarshal(body, &b)

		first := ""
		for _, t := range b.Threads {
			if !t.Triggered {
				continue
			}
			if len(t.Frames) > 0 {
				first = t.Frames[0].Symbol
			}
			break
		}
		var parts []string
		if b.Exception.Type != "" {
			parts = append(parts, b.Exception.Type)
		}
		if b.Exception.Signal != "" {
			parts = append(parts, "("+b.Exception.Signal+")")
		}
		if first != "" {
			parts = append(parts, "at "+first)
		}
		if len(parts) == 0 {
			parts = []string{"crashed"}
		}
		return strings.Join(parts, " "), c.path
	}
	return "", ""
}
