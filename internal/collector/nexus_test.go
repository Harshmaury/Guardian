// @guardian-project: guardian
// @guardian-path: internal/collector/nexus_test.go
package collector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"log"
	"os"
)

func testLogger() *log.Logger {
	return log.New(os.Stderr, "", 0)
}

func TestNexusCollector_CollectServices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services":
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": []map[string]any{
					{"id": "atlas-daemon", "name": "atlas-daemon", "project": "atlas",
						"desired_state": "running", "actual_state": "running", "fail_count": 0},
					{"id": "forge-daemon", "name": "forge-daemon", "project": "forge",
						"desired_state": "running", "actual_state": "maintenance", "fail_count": 3},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c := NewNexusCollector(server.URL, "", testLogger())
	svcs := c.CollectServices(t.Context(), "test-trace")

	if len(svcs) != 2 {
		t.Fatalf("expected 2 services, got %d", len(svcs))
	}
	if svcs[0].ID != "atlas-daemon" {
		t.Errorf("expected atlas-daemon, got %s", svcs[0].ID)
	}
	if svcs[1].ActualState != "maintenance" {
		t.Errorf("expected maintenance, got %s", svcs[1].ActualState)
	}
	if svcs[1].ProjectID != "forge" {
		t.Errorf("expected forge project, got %s", svcs[1].ProjectID)
	}
}

func TestNexusCollector_CollectProjects_BugFix(t *testing.T) {
	// This test verifies the json:"id" bug fix.
	// Before fix: json:"ProjectID" — projects always returned empty, G-007/G-008 never fired.
	// After fix: accord.ProjectDTO uses json:"id" — correctly maps Nexus response.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/projects" {
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": []map[string]any{
					{"id": "atlas", "name": "atlas"},
					{"id": "forge", "name": "forge"},
				},
			})
		}
	}))
	defer server.Close()

	c := NewNexusCollector(server.URL, "", testLogger())
	projects := c.CollectProjects(t.Context(), "")

	if len(projects) != 2 {
		t.Fatalf("bug: expected 2 projects, got %d — json field mapping broken", len(projects))
	}
	if projects[0].ID != "atlas" {
		t.Errorf("expected atlas, got %q", projects[0].ID)
	}
}

func TestNexusCollector_CollectEvents_CursorAdvances(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		since := r.URL.Query().Get("since")
		var events []map[string]any
		if since == "0" || since == "" {
			events = []map[string]any{
				{"id": 1, "type": "SERVICE_STARTED", "created_at": "2026-01-01T00:00:00Z"},
				{"id": 2, "type": "SERVICE_CRASHED", "created_at": "2026-01-01T00:00:01Z"},
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": events})
	}))
	defer server.Close()

	c := NewNexusCollector(server.URL, "", testLogger())

	evs := c.Collect(t.Context(), "")
	if len(evs) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evs))
	}
	if c.lastEventID != 2 {
		t.Errorf("expected cursor=2, got %d", c.lastEventID)
	}

	// Second call — cursor should be 2, server returns empty
	evs2 := c.Collect(t.Context(), "")
	if len(evs2) != 0 {
		t.Errorf("expected 0 new events on second call, got %d", len(evs2))
	}
}

func TestNexusCollector_Unreachable(t *testing.T) {
	c := NewNexusCollector("http://127.0.0.1:19999", "", testLogger())
	// Must return nil, not panic
	svcs := c.CollectServices(t.Context(), "")
	if svcs != nil {
		t.Error("expected nil on unreachable server")
	}
	projs := c.CollectProjects(t.Context(), "")
	if projs != nil {
		t.Error("expected nil on unreachable server")
	}
}
