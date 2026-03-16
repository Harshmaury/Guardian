// @guardian-project: guardian
// @guardian-path: internal/policy/model.go
// Package policy defines Guardian finding types and rule constants.
package policy

import "time"

// Rule IDs — all Guardian policy rules.
const (
	RuleRepeatedDenials   = "G-001"
	RuleUnverifiedTargets = "G-002"
	RuleHighFailureRate   = "G-003"
	RuleServiceCrashes    = "G-004"
	RuleUnverifiedProjects = "G-005"
)

// Severity levels for findings.
const (
	SeverityWarning = "warning"
	SeverityError   = "error"
)

// Finding is a single policy violation or anomaly detected by Guardian.
type Finding struct {
	RuleID    string    `json:"rule_id"`
	Severity  string    `json:"severity"`
	Target    string    `json:"target"`
	Message   string    `json:"message"`
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// Summary is the aggregate count of findings by severity.
type Summary struct {
	Total    int `json:"total"`
	Warnings int `json:"warnings"`
	Errors   int `json:"errors"`
}

// Report is the full Guardian findings report returned by GET /guardian/findings.
type Report struct {
	Findings    []*Finding `json:"findings"`
	Summary     Summary    `json:"summary"`
	EvaluatedAt time.Time  `json:"evaluated_at"`
}

// NewReport builds a Report from a slice of findings.
func NewReport(findings []*Finding) *Report {
	r := &Report{
		Findings:    findings,
		EvaluatedAt: time.Now().UTC(),
	}
	if r.Findings == nil {
		r.Findings = []*Finding{}
	}
	for _, f := range findings {
		r.Summary.Total++
		if f.Severity == SeverityError {
			r.Summary.Errors++
		} else {
			r.Summary.Warnings++
		}
	}
	return r
}
