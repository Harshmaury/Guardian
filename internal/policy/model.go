// @guardian-project: guardian
// @guardian-path: internal/policy/model.go
// Package policy defines Guardian finding types and rule constants.
package policy

import "time"

// Rule IDs — platform health rules (G-001..G-010).
const (
	RuleRepeatedDenials       = "G-001"
	RuleUnverifiedTargets     = "G-002"
	RuleHighFailureRate       = "G-003"
	RuleServiceCrashes        = "G-004"
	RuleUnverifiedProjects    = "G-005"
	RuleServiceMaintenance    = "G-006" // service desired=running stuck in maintenance
	RuleNeverBuilt            = "G-007" // registered project with zero successful builds
	RuleNoService             = "G-008" // registered project with no service entry
	RuleUnatributedExecution  = "G-009" // execution with no identity actor (ADR-042)
	RuleInsecureMode          = "G-010" // platform running in insecure mode (ADR-044)
	RuleSkipEnforceBypass  = "G-019" // --skip-enforce used: Arbiter gate bypassed (ADR-047)
)

// Rule IDs — project-level governance rules (G-011..G-020).
// These evaluate individual project health rather than platform health.
const (
	RuleCrashLoop           = "G-011" // service crashed 5+ times in 30 min after recovery
	RuleBeyondRecovery      = "G-012" // service in maintenance after Sentinel actuator exhausted
	RuleHighRestartFreq     = "G-013" // service stopped+started 4+ times in 60 min (not crashes)
	RuleBuildNeverSucceeded = "G-014" // 5+ builds attempted, zero succeeded in last 24h
	RuleExecutionLoop       = "G-015" // same intent executed 3+ times in 10 min, no state change
	RuleStuckDenied         = "G-016" // last execution denied, no success in 48h
	RuleNeverUsed           = "G-017" // registered 7+ days ago, never executed, never running
	RuleStaleProject        = "G-018" // no execution in 30 days, service desired=stopped
	RuleSkipEnforceBypass  = "G-019" // --skip-enforce used: Arbiter gate bypassed (ADR-047)
)

// Severity levels for findings.
const (
	SeverityInfo    = "info"    // informational — no active problem
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
	Info     int `json:"info"`
}

// Report is the full Guardian findings report returned by GET /guardian/findings.
type Report struct {
	Findings    []*Finding    `json:"findings"`
	Summary     Summary       `json:"summary"`
	EvaluatedAt time.Time     `json:"evaluated_at"`
	IsStale     bool          `json:"is_stale"`
	StaleAfter  time.Duration `json:"stale_after_seconds"`
	StaleSince  *time.Time    `json:"stale_since,omitempty"`
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
		switch f.Severity {
		case SeverityError:
			r.Summary.Errors++
		case SeverityInfo:
			r.Summary.Info++
		default:
			r.Summary.Warnings++
		}
	}
	return r
}
