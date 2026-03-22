// @path: internal/collector/cursor.go
// sinceID cursor persistence — Fix 4 (pre-Relay stabilisation, ADR-044).
// Persists lastEventID to ~/.nexus/state/<service>-cursor.json
// so the collector restarts from the correct position after a crash/restart.
// Without this, every restart re-evaluates from event 0, producing false findings
// (e.g. G-007 "never built" fires for projects with existing successful builds).
//
// Atomic write: write to .tmp then rename — no partial reads on crash.
// Fail-safe: read errors return 0 (safe restart), write errors are silent.
package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type cursorFile struct {
	LastEventID int64 `json:"last_event_id"`
}

// loadCursor reads the persisted lastEventID for serviceName from disk.
// Returns 0 if the file does not exist or cannot be parsed.
func loadCursor(serviceName string) int64 {
	data, err := os.ReadFile(cursorPath(serviceName))
	if err != nil {
		return 0
	}
	var cf cursorFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return 0
	}
	return cf.LastEventID
}

// saveCursor atomically persists lastEventID to disk.
func saveCursor(serviceName string, id int64) {
	path := cursorPath(serviceName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	data, _ := json.Marshal(cursorFile{LastEventID: id})
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	os.Rename(tmp, path)
}

func cursorPath(serviceName string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", serviceName+"-cursor.json")
	}
	return filepath.Join(home, ".nexus", "state", serviceName+"-cursor.json")
}
