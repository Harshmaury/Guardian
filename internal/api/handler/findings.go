// @guardian-project: guardian
// @guardian-path: internal/api/handler/findings.go
// ReportStore tracks staleness — Fix 3 (audit).
// IsStale=true when no evaluation in > 2× poll interval (2 min).
package handler

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Harshmaury/Guardian/internal/policy"
)

const DefaultStaleWindow = 2 * time.Minute

type ReportStore struct {
	mu          sync.RWMutex
	report      *policy.Report
	lastUpdated time.Time
	staleWindow time.Duration
}

func NewReportStore() *ReportStore {
	return &ReportStore{staleWindow: DefaultStaleWindow}
}

func (s *ReportStore) Set(r *policy.Report) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.StaleAfter = s.staleWindow
	r.IsStale = false
	r.StaleSince = nil
	s.report = r
	s.lastUpdated = time.Now()
}

func (s *ReportStore) Get() *policy.Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.report == nil {
		return policy.NewReport(nil)
	}
	age := time.Since(s.lastUpdated)
	s.report.IsStale = age > s.staleWindow
	if s.report.IsStale && s.report.StaleSince == nil {
		t := s.lastUpdated.Add(s.staleWindow)
		s.report.StaleSince = &t
	}
	if !s.report.IsStale {
		s.report.StaleSince = nil
	}
	return s.report
}

type FindingsHandler struct{ store *ReportStore }

func NewFindingsHandler(s *ReportStore) *FindingsHandler { return &FindingsHandler{store: s} }

func (h *FindingsHandler) All(w http.ResponseWriter, r *http.Request) {
	respondOK(w, h.store.Get())
}

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
