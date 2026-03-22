// @guardian-project: guardian
// @guardian-path: internal/policy/engine.go
// Engine evaluates all Guardian policy rules against collected data.
// Rules are stateless — evaluated fresh on every collection cycle.
package policy

import (
	"fmt"
	"time"

	canonevents "github.com/Harshmaury/Canon/events"
)

// ── INPUT TYPES ───────────────────────────────────────────────────────────────

// ExecutionRecord is a Forge history entry consumed by the engine.
type ExecutionRecord struct {
	Target    string
	Status    string // "success" | "failure" | "denied"
	ActorSub  string // Gate subject — empty = anonymous (ADR-042)
	StartedAt time.Time
}

// TopologyNode is a Navigator graph node consumed by the engine.
type TopologyNode struct {
	ID     string
	Status string // "verified" | "unverified"
}

// NexusEvent is a Nexus event entry consumed by the engine.
type NexusEvent struct {
	Type      string
	CreatedAt time.Time
}

// ServiceRecord is a Nexus service entry consumed by G-006 and G-008.
type ServiceRecord struct {
	ID           string
	ProjectID    string
	DesiredState string
	ActualState  string
}

// ProjectRecord is a registered Nexus project consumed by G-007 and G-008.
type ProjectRecord struct {
	ID string
}

// ── ENGINE ────────────────────────────────────────────────────────────────────

// Engine evaluates policy rules and produces findings.
type Engine struct {
	requireIdentity bool // set via GUARDIAN_REQUIRE_IDENTITY env (ADR-042)
}

// NewEngine creates an Engine.
// requireIdentity enables G-009 — set from GUARDIAN_REQUIRE_IDENTITY env var.
func NewEngine(requireIdentity bool) *Engine { return &Engine{requireIdentity: requireIdentity} }

// Evaluate runs all rules and returns a Report.
func (e *Engine) Evaluate(
	executions []ExecutionRecord,
	nodes []TopologyNode,
	events []NexusEvent,
	services []ServiceRecord,
	projects []ProjectRecord,
) *Report {
	var findings []*Finding

	findings = append(findings, e.ruleRepeatedDenials(executions)...)
	findings = append(findings, e.ruleUnverifiedTargets(executions, nodes)...)
	findings = append(findings, e.ruleHighFailureRate(executions)...)
	findings = append(findings, e.ruleServiceCrashes(events)...)
	findings = append(findings, e.ruleUnverifiedProjects(nodes)...)
	findings = append(findings, e.ruleServiceMaintenance(services)...)
	findings = append(findings, e.ruleNeverBuilt(executions, projects)...)
	findings = append(findings, e.ruleNoService(services, projects)...)
	findings = append(findings, e.ruleUnattributedExecution(executions)...)

	return NewReport(findings)
}

// G-001: same target denied 3+ times in last 10 minutes.
func (e *Engine) ruleRepeatedDenials(execs []ExecutionRecord) []*Finding {
	cutoff := time.Now().UTC().Add(-10 * time.Minute)
	counts := map[string][]time.Time{}

	for _, ex := range execs {
		if ex.Status == "denied" && ex.StartedAt.After(cutoff) {
			counts[ex.Target] = append(counts[ex.Target], ex.StartedAt)
		}
	}

	var findings []*Finding
	for target, times := range counts {
		if len(times) >= 3 {
			first, last := times[0], times[len(times)-1]
			for _, t := range times {
				if t.Before(first) {
					first = t
				}
				if t.After(last) {
					last = t
				}
			}
			findings = append(findings, &Finding{
				RuleID:    RuleRepeatedDenials,
				Severity:  SeverityWarning,
				Target:    target,
				Message:   fmt.Sprintf("project denied %d times in last 10 minutes — add nexus.yaml", len(times)),
				Count:     len(times),
				FirstSeen: first,
				LastSeen:  last,
			})
		}
	}
	return findings
}

// G-002: any execution against an unverified project.
func (e *Engine) ruleUnverifiedTargets(execs []ExecutionRecord, nodes []TopologyNode) []*Finding {
	unverified := map[string]bool{}
	for _, n := range nodes {
		if n.Status == "unverified" {
			unverified[n.ID] = true
		}
	}

	seen := map[string]bool{}
	var findings []*Finding
	for _, ex := range execs {
		if unverified[ex.Target] && !seen[ex.Target] {
			seen[ex.Target] = true
			findings = append(findings, &Finding{
				RuleID:    RuleUnverifiedTargets,
				Severity:  SeverityWarning,
				Target:    ex.Target,
				Message:   fmt.Sprintf("commands executed against unverified project %q — add nexus.yaml", ex.Target),
				Count:     1,
				FirstSeen: ex.StartedAt,
				LastSeen:  ex.StartedAt,
			})
		}
	}
	return findings
}

// G-003: >50% failure rate for a target in last 20 executions.
func (e *Engine) ruleHighFailureRate(execs []ExecutionRecord) []*Finding {
	type stats struct {
		total, failed int
		first, last   time.Time
	}
	byTarget := map[string]*stats{}

	for _, ex := range execs {
		s := byTarget[ex.Target]
		if s == nil {
			s = &stats{first: ex.StartedAt, last: ex.StartedAt}
			byTarget[ex.Target] = s
		}
		s.total++
		if ex.Status == "failure" {
			s.failed++
		}
		if ex.StartedAt.Before(s.first) {
			s.first = ex.StartedAt
		}
		if ex.StartedAt.After(s.last) {
			s.last = ex.StartedAt
		}
	}

	var findings []*Finding
	for target, s := range byTarget {
		if s.total >= 3 && s.failed*100/s.total > 50 {
			findings = append(findings, &Finding{
				RuleID:    RuleHighFailureRate,
				Severity:  SeverityError,
				Target:    target,
				Message:   fmt.Sprintf("project %q has %d%% failure rate (%d/%d executions)", target, s.failed*100/s.total, s.failed, s.total),
				Count:     s.failed,
				FirstSeen: s.first,
				LastSeen:  s.last,
			})
		}
	}
	return findings
}

// G-004: SERVICE_CRASHED event in last 5 minutes.
func (e *Engine) ruleServiceCrashes(events []NexusEvent) []*Finding {
	cutoff := time.Now().UTC().Add(-5 * time.Minute)
	var crashes []time.Time
	for _, ev := range events {
		if ev.Type == canonevents.EventServiceCrashed && ev.CreatedAt.After(cutoff) {
			crashes = append(crashes, ev.CreatedAt)
		}
	}
	if len(crashes) == 0 {
		return nil
	}
	first, last := crashes[0], crashes[0]
	for _, t := range crashes {
		if t.Before(first) {
			first = t
		}
		if t.After(last) {
			last = t
		}
	}
	return []*Finding{{
		RuleID:    RuleServiceCrashes,
		Severity:  SeverityError,
		Target:    "system",
		Message:   fmt.Sprintf("%d service crash(es) detected in last 5 minutes", len(crashes)),
		Count:     len(crashes),
		FirstSeen: first,
		LastSeen:  last,
	}}
}

// G-005: projects in graph with status=unverified.
func (e *Engine) ruleUnverifiedProjects(nodes []TopologyNode) []*Finding {
	var findings []*Finding
	now := time.Now().UTC()
	for _, n := range nodes {
		if n.Status == "unverified" {
			findings = append(findings, &Finding{
				RuleID:    RuleUnverifiedProjects,
				Severity:  SeverityWarning,
				Target:    n.ID,
				Message:   fmt.Sprintf("project %q has no valid nexus.yaml — add descriptor to enable full platform integration", n.ID),
				Count:     1,
				FirstSeen: now,
				LastSeen:  now,
			})
		}
	}
	return findings
}

// G-006: service desired=running but actual=maintenance.
func (e *Engine) ruleServiceMaintenance(services []ServiceRecord) []*Finding {
	var findings []*Finding
	now := time.Now().UTC()
	for _, svc := range services {
		if svc.DesiredState == "running" && svc.ActualState == "maintenance" {
			findings = append(findings, &Finding{
				RuleID:    RuleServiceMaintenance,
				Severity:  SeverityError,
				Target:    svc.ProjectID,
				Message:   fmt.Sprintf("service %q is stuck in maintenance — run: engx services reset %s", svc.ID, svc.ID),
				Count:     1,
				FirstSeen: now,
				LastSeen:  now,
			})
		}
	}
	return findings
}

// G-007: registered project has zero successful builds ever.
func (e *Engine) ruleNeverBuilt(execs []ExecutionRecord, projects []ProjectRecord) []*Finding {
	succeeded := map[string]bool{}
	attempted := map[string]bool{}
	for _, ex := range execs {
		attempted[ex.Target] = true
		if ex.Status == "success" {
			succeeded[ex.Target] = true
		}
	}
	var findings []*Finding
	now := time.Now().UTC()
	for _, p := range projects {
		if attempted[p.ID] && !succeeded[p.ID] {
			findings = append(findings, &Finding{
				RuleID:    RuleNeverBuilt,
				Severity:  SeverityWarning,
				Target:    p.ID,
				Message:   fmt.Sprintf("project %q has been attempted but never built successfully", p.ID),
				Count:     1,
				FirstSeen: now,
				LastSeen:  now,
			})
		}
	}
	return findings
}

// G-008: registered project has no service entry in Nexus.
func (e *Engine) ruleNoService(services []ServiceRecord, projects []ProjectRecord) []*Finding {
	hasService := map[string]bool{}
	for _, svc := range services {
		hasService[svc.ProjectID] = true
	}
	var findings []*Finding
	now := time.Now().UTC()
	for _, p := range projects {
		if !hasService[p.ID] {
			findings = append(findings, &Finding{
				RuleID:    RuleNoService,
				Severity:  SeverityWarning,
				Target:    p.ID,
				Message:   fmt.Sprintf("project %q is registered but has no service — run: engx init %s --register", p.ID, p.ID),
				Count:     1,
				FirstSeen: now,
				LastSeen:  now,
			})
		}
	}
	return findings
}

// G-009: execution with no identity actor — anonymous execution detected (ADR-042).
// Only fires when GUARDIAN_REQUIRE_IDENTITY=true is set in the environment.
// Fail-open by default — does not block execution, only flags it.
func (e *Engine) ruleUnattributedExecution(execs []ExecutionRecord) []*Finding {
	if !e.requireIdentity {
		return nil
	}
	seen := map[string]bool{}
	var findings []*Finding
	now := time.Now().UTC()
	for _, ex := range execs {
		if ex.ActorSub == "" && !seen[ex.Target] {
			seen[ex.Target] = true
			findings = append(findings, &Finding{
				RuleID:    RuleUnatributedExecution,
				Severity:  SeverityWarning,
				Target:    ex.Target,
				Message:   fmt.Sprintf("execution against %q has no identity actor — run: engx login", ex.Target),
				Count:     1,
				FirstSeen: now,
				LastSeen:  now,
			})
		}
	}
	return findings
}
