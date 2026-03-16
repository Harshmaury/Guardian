// @guardian-project: guardian
// @guardian-path: internal/api/handler/findings.go
// FindingsHandler serves GET /guardian/findings endpoints.
package handler

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/Harshmaury/Guardian/internal/policy"
)

// ReportStore holds the latest evaluated report in memory.
type ReportStore struct {
	mu     sync.RWMutex
	report *policy.Report
}

// NewReportStore creates an empty ReportStore.
func NewReportStore() *ReportStore { return &ReportStore{} }

// Set updates the stored report atomically.
func (s *ReportStore) Set(r *policy.Report) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.report = r
}

// Get returns the latest report, or an empty one if none yet.
func (s *ReportStore) Get() *policy.Report {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.report == nil {
		return policy.NewReport(nil)
	}
	return s.report
}

// FindingsHandler handles GET /guardian/findings routes.
type FindingsHandler struct {
	store *ReportStore
}

// NewFindingsHandler creates a FindingsHandler.
func NewFindingsHandler(s *ReportStore) *FindingsHandler {
	return &FindingsHandler{store: s}
}

// All handles GET /guardian/findings — returns all findings.
func (h *FindingsHandler) All(w http.ResponseWriter, r *http.Request) {
	respondOK(w, h.store.Get())
}

// ByRule handles GET /guardian/findings/:rule_id — findings for one rule.
func (h *FindingsHandler) ByRule(w http.ResponseWriter, r *http.Request) {
	ruleID := r.PathValue("rule_id")
	report := h.store.Get()

	var filtered []*policy.Finding
	for _, f := range report.Findings {
		if f.RuleID == ruleID {
			filtered = append(filtered, f)
		}
	}
	respondOK(w, policy.NewReport(filtered))
}

// ── RESPONSE HELPERS ──────────────────────────────────────────────────────────

type apiResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func respondOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(apiResponse{OK: true, Data: data}) //nolint:errcheck
}
