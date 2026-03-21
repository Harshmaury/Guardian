// @guardian-project: guardian
// @guardian-path: internal/api/handler/findings_test.go
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Harshmaury/Guardian/internal/policy"
)

func makeReport(findings ...*policy.Finding) *policy.Report {
	return policy.NewReport(findings)
}

func makeFinding(ruleID, severity, target string) *policy.Finding {
	return &policy.Finding{
		RuleID:    ruleID,
		Severity:  severity,
		Target:    target,
		Message:   "test finding",
		Count:     1,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}
}

func TestFindingsHandler_All_Empty(t *testing.T) {
	store := NewReportStore()
	h := NewFindingsHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/guardian/findings", nil)
	w := httptest.NewRecorder()
	h.All(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		OK   bool           `json:"ok"`
		Data *policy.Report `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Error("expected ok=true")
	}
	if resp.Data.Summary.Total != 0 {
		t.Errorf("expected 0 findings, got %d", resp.Data.Summary.Total)
	}
}

func TestFindingsHandler_All_WithFindings(t *testing.T) {
	store := NewReportStore()
	store.Set(makeReport(
		makeFinding("G-003", "error", "fake-project"),
		makeFinding("G-008", "warning", "test-proj"),
	))
	h := NewFindingsHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/guardian/findings", nil)
	w := httptest.NewRecorder()
	h.All(w, req)

	var resp struct {
		OK   bool           `json:"ok"`
		Data *policy.Report `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Data.Summary.Total != 2 {
		t.Errorf("expected 2 findings, got %d", resp.Data.Summary.Total)
	}
	if resp.Data.Summary.Errors != 1 {
		t.Errorf("expected 1 error, got %d", resp.Data.Summary.Errors)
	}
	if resp.Data.Summary.Warnings != 1 {
		t.Errorf("expected 1 warning, got %d", resp.Data.Summary.Warnings)
	}
}

func TestFindingsHandler_ByRule(t *testing.T) {
	store := NewReportStore()
	store.Set(makeReport(
		makeFinding("G-003", "error", "atlas"),
		makeFinding("G-003", "error", "forge"),
		makeFinding("G-008", "warning", "orphan"),
	))
	h := NewFindingsHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/guardian/findings/G-003", nil)
	req.SetPathValue("rule_id", "G-003")
	w := httptest.NewRecorder()
	h.ByRule(w, req)

	var resp struct {
		Data *policy.Report `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Data.Summary.Total != 2 {
		t.Errorf("ByRule G-003: expected 2, got %d", resp.Data.Summary.Total)
	}
}

func TestFindingsHandler_ByRule_NotFound(t *testing.T) {
	store := NewReportStore()
	store.Set(makeReport(makeFinding("G-003", "error", "atlas")))
	h := NewFindingsHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/guardian/findings/G-999", nil)
	req.SetPathValue("rule_id", "G-999")
	w := httptest.NewRecorder()
	h.ByRule(w, req)

	var resp struct {
		Data *policy.Report `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.Summary.Total != 0 {
		t.Error("expected 0 findings for unknown rule")
	}
}

func TestReportStore_ConcurrentAccess(t *testing.T) {
	store := NewReportStore()
	done := make(chan struct{})

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			store.Set(makeReport(makeFinding("G-001", "warning", "x")))
		}
		close(done)
	}()

	// Reader goroutines
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = store.Get()
			}
		}()
	}
	<-done
}
