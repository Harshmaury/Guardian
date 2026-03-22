// @guardian-project: guardian
// @guardian-path: internal/policy/engine.go
// Engine evaluates all Guardian policy rules against collected data.
// Rules are stateless — evaluated fresh on every collection cycle.
package policy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	canonevents "github.com/Harshmaury/Canon/events"
)

// ── INPUT TYPES ───────────────────────────────────────────────────────────────

// ExecutionRecord is a Forge history entry consumed by the engine.
type ExecutionRecord struct {
	Target     string
	Intent     string    // "build" | "run" | "test" | "deploy"
	Status     string    // "success" | "failure" | "denied"
	ActorSub   string    // Gate subject — empty = anonymous (ADR-042)
	StartedAt  time.Time
	FinishedAt time.Time
}

// TopologyNode is a Navigator graph node consumed by the engine.
type TopologyNode struct {
	ID     string
	Status string // "verified" | "unverified"
}

// NexusEvent is a Nexus event entry consumed by the engine.
type NexusEvent struct {
	Type      string
	ServiceID string    // which service emitted this event
	CreatedAt time.Time
}

// ServiceRecord is a Nexus service entry consumed by G-006 and G-008.
type ServiceRecord struct {
	ID           string
	ProjectID    string
	DesiredState string
	ActualState  string
	FailCount    int // current failure counter from Nexus recovery controller
}

// ProjectRecord is a registered Nexus project consumed by G-007 and G-008.
type ProjectRecord struct {
	ID string
}

// ── ENGINE ────────────────────────────────────────────────────────────────────

// Engine evaluates policy rules and produces findings.
type Engine struct {
	requireIdentity bool   // set via GUARDIAN_REQUIRE_IDENTITY env (ADR-042)
	nexusAddr       string // used by G-010 to probe /system/mode (ADR-044)
	httpClient      *http.Client
}

// NewEngine creates an Engine.
// requireIdentity enables G-009 — set from GUARDIAN_REQUIRE_IDENTITY env var.
// nexusAddr is the Nexus HTTP address for G-010 mode probing.
func NewEngine(requireIdentity bool, nexusAddr string) *Engine {
	return &Engine{
		requireIdentity: requireIdentity,
		nexusAddr:       nexusAddr,
		httpClient:      &http.Client{Timeout: 2 * time.Second},
	}
}

// Evaluate runs all rules and returns a Report.
func (e *Engine) Evaluate(
	executions []ExecutionRecord,
	nodes []TopologyNode,
	events []NexusEvent,
	services []ServiceRecord,
	projects []ProjectRecord,
) *Report {
	var findings []*Finding

	// ── Platform health rules (G-001..G-010) ─────────────────────────────────
	findings = append(findings, e.ruleRepeatedDenials(executions)...)
	findings = append(findings, e.ruleUnverifiedTargets(executions, nodes)...)
	findings = append(findings, e.ruleHighFailureRate(executions)...)
	findings = append(findings, e.ruleServiceCrashes(events)...)
	findings = append(findings, e.ruleUnverifiedProjects(nodes)...)
	findings = append(findings, e.ruleServiceMaintenance(services)...)
	findings = append(findings, e.ruleNeverBuilt(executions, projects)...)
	findings = append(findings, e.ruleNoService(services, projects)...)
	findings = append(findings, e.ruleUnattributedExecution(executions)...)
	findings = append(findings, e.ruleInsecureMode()...)

	// ── Project governance rules (G-011..G-018) ───────────────────────────────
	findings = append(findings, e.ruleCrashLoop(services)...)
	findings = append(findings, e.ruleBeyondRecovery(services)...)
	findings = append(findings, e.ruleHighRestartFrequency(events, services)...)
	findings = append(findings, e.ruleBuildNeverSucceeded(executions)...)
	findings = append(findings, e.ruleExecutionLoop(executions)...)
	findings = append(findings, e.ruleStuckDenied(executions)...)
	findings = append(findings, e.ruleNeverUsed(executions, services, projects)...)
	findings = append(findings, e.ruleStaleProject(executions, services)...)

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

// G-010: platform running in insecure mode (ADR-044).
// Polls GET /system/mode on Nexus every evaluation cycle.
// Clears automatically when mode returns to degraded or full.
func (e *Engine) ruleInsecureMode() []*Finding {
	if e.nexusAddr == "" {
		return nil
	}
	resp, err := e.httpClient.Get(e.nexusAddr + "/system/mode")
	if err != nil {
		return nil // Nexus unreachable — skip silently, not a G-010 trigger
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var result struct {
		Data struct {
			Mode string `json:"mode"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	if result.Data.Mode != "insecure" {
		return nil
	}
	now := time.Now().UTC()
	return []*Finding{{
		RuleID:   RuleInsecureMode,
		Severity: SeverityWarning,
		Target:   "platform",
		Message:  "platform running in insecure mode — identity disabled. Start Gate or set GUARDIAN_REQUIRE_IDENTITY=false to suppress.",
		Count:    1,
		FirstSeen: now,
		LastSeen:  now,
	}}
}

// ── Project governance rules (G-011..G-018) ───────────────────────────────────

// G-011: service crashed 5+ times in last 30 minutes — crash loop.
// Distinct from G-004 (any crash in 5 min): this fires when the pattern
// persists well past the initial back-off window.
func (e *Engine) ruleCrashLoop(services []ServiceRecord) []*Finding {
	const threshold = 5
	var findings []*Finding
	now := time.Now().UTC()
	for _, svc := range services {
		if svc.DesiredState != "running" {
			continue
		}
		if svc.FailCount >= threshold {
			findings = append(findings, &Finding{
				RuleID:    RuleCrashLoop,
				Severity:  SeverityError,
				Target:    svc.ProjectID,
				Message:   fmt.Sprintf("service %q is in a crash loop — %d failures recorded. Check logs: engx logs %s", svc.ID, svc.FailCount, svc.ID),
				Count:     svc.FailCount,
				FirstSeen: now,
				LastSeen:  now,
			})
		}
	}
	return findings
}

// G-012: service desired=running, in maintenance, fail count at max threshold.
// The recovery system has given up — manual intervention is required.
func (e *Engine) ruleBeyondRecovery(services []ServiceRecord) []*Finding {
	const maxFails = 3
	var findings []*Finding
	now := time.Now().UTC()
	for _, svc := range services {
		if svc.DesiredState == "running" &&
			svc.ActualState == "maintenance" &&
			svc.FailCount >= maxFails {
			findings = append(findings, &Finding{
				RuleID:    RuleBeyondRecovery,
				Severity:  SeverityError,
				Target:    svc.ProjectID,
				Message:   fmt.Sprintf("service %q is beyond automatic recovery (%d failures). Reset manually: engx services reset %s", svc.ID, svc.FailCount, svc.ID),
				Count:     svc.FailCount,
				FirstSeen: now,
				LastSeen:  now,
			})
		}
	}
	return findings
}

// G-013: service restarted 4+ times in 60 minutes via platform commands (not crashes).
// Indicates an unstable deployment loop or aggressive automation.
func (e *Engine) ruleHighRestartFrequency(events []NexusEvent, services []ServiceRecord) []*Finding {
	const threshold = 4
	cutoff := time.Now().UTC().Add(-60 * time.Minute)

	// Build project → service map for lookup.
	svcToProject := map[string]string{}
	for _, s := range services {
		svcToProject[s.ID] = s.ProjectID
	}

	// Count SERVICE_STARTED events per service in the window.
	counts := map[string][]time.Time{}
	for _, ev := range events {
		if ev.Type == "SERVICE_STARTED" && ev.CreatedAt.After(cutoff) && ev.ServiceID != "" {
			counts[ev.ServiceID] = append(counts[ev.ServiceID], ev.CreatedAt)
		}
	}

	var findings []*Finding
	seen := map[string]bool{}
	for svcID, times := range counts {
		projectID := svcToProject[svcID]
		if projectID == "" || seen[projectID] {
			continue
		}
		if len(times) >= threshold {
			seen[projectID] = true
			first, last := times[0], times[len(times)-1]
			for _, t := range times {
				if t.Before(first) { first = t }
				if t.After(last) { last = t }
			}
			findings = append(findings, &Finding{
				RuleID:    RuleHighRestartFreq,
				Severity:  SeverityWarning,
				Target:    projectID,
				Message:   fmt.Sprintf("service %q restarted %d times in the last hour — check automation triggers or deployment scripts", svcID, len(times)),
				Count:     len(times),
				FirstSeen: first,
				LastSeen:  last,
			})
		}
	}
	return findings
}

// G-014: 5+ build executions in last 24 hours, zero successes.
// The developer is actively trying but the build is consistently broken.
func (e *Engine) ruleBuildNeverSucceeded(execs []ExecutionRecord) []*Finding {
	const minAttempts = 5
	cutoff := time.Now().UTC().Add(-24 * time.Hour)

	type stats struct {
		total, succeeded int
		first, last      time.Time
	}
	byTarget := map[string]*stats{}

	for _, ex := range execs {
		if ex.Intent != "build" || ex.StartedAt.Before(cutoff) {
			continue
		}
		s := byTarget[ex.Target]
		if s == nil {
			s = &stats{first: ex.StartedAt, last: ex.StartedAt}
			byTarget[ex.Target] = s
		}
		s.total++
		if ex.Status == "success" {
			s.succeeded++
		}
		if ex.StartedAt.Before(s.first) { s.first = ex.StartedAt }
		if ex.StartedAt.After(s.last) { s.last = ex.StartedAt }
	}

	var findings []*Finding
	for target, s := range byTarget {
		if s.total >= minAttempts && s.succeeded == 0 {
			findings = append(findings, &Finding{
				RuleID:    RuleBuildNeverSucceeded,
				Severity:  SeverityError,
				Target:    target,
				Message:   fmt.Sprintf("project %q has %d failed build attempts in 24h with no success — run: engx check %s", target, s.total, target),
				Count:     s.total,
				FirstSeen: s.first,
				LastSeen:  s.last,
			})
		}
	}
	return findings
}

// G-015: same intent executed 3+ times in 10 minutes — execution loop.
// Indicates a retry loop, broken automation trigger, or CI misconfiguration.
func (e *Engine) ruleExecutionLoop(execs []ExecutionRecord) []*Finding {
	const threshold = 3
	cutoff := time.Now().UTC().Add(-10 * time.Minute)

	type key struct{ target, intent string }
	counts := map[key][]time.Time{}

	for _, ex := range execs {
		if ex.StartedAt.After(cutoff) {
			k := key{ex.Target, ex.Intent}
			counts[k] = append(counts[k], ex.StartedAt)
		}
	}

	var findings []*Finding
	seen := map[string]bool{}
	for k, times := range counts {
		if len(times) >= threshold && !seen[k.target] {
			seen[k.target] = true
			first, last := times[0], times[len(times)-1]
			for _, t := range times {
				if t.Before(first) { first = t }
				if t.After(last) { last = t }
			}
			findings = append(findings, &Finding{
				RuleID:    RuleExecutionLoop,
				Severity:  SeverityWarning,
				Target:    k.target,
				Message:   fmt.Sprintf("project %q has had %q executed %d times in 10 minutes — check automation triggers", k.target, k.intent, len(times)),
				Count:     len(times),
				FirstSeen: first,
				LastSeen:  last,
			})
		}
	}
	return findings
}

// G-016: last execution was denied AND no successful execution in 48 hours.
// The project is stuck in an unexecutable state.
func (e *Engine) ruleStuckDenied(execs []ExecutionRecord) []*Finding {
	cutoff48h := time.Now().UTC().Add(-48 * time.Hour)

	// Latest execution and last success per target.
	type rec struct {
		lastStatus    string
		lastAt        time.Time
		lastSuccessAt time.Time
	}
	byTarget := map[string]*rec{}

	for _, ex := range execs {
		r := byTarget[ex.Target]
		if r == nil {
			r = &rec{}
			byTarget[ex.Target] = r
		}
		if ex.StartedAt.After(r.lastAt) {
			r.lastAt = ex.StartedAt
			r.lastStatus = ex.Status
		}
		if ex.Status == "success" && ex.StartedAt.After(r.lastSuccessAt) {
			r.lastSuccessAt = ex.StartedAt
		}
	}

	var findings []*Finding
	now := time.Now().UTC()
	for target, r := range byTarget {
		if r.lastStatus == "denied" && r.lastSuccessAt.Before(cutoff48h) {
			findings = append(findings, &Finding{
				RuleID:    RuleStuckDenied,
				Severity:  SeverityWarning,
				Target:    target,
				Message:   fmt.Sprintf("project %q last execution was denied and has had no success in 48h — check: engx check %s", target, target),
				Count:     1,
				FirstSeen: r.lastAt,
				LastSeen:  now,
			})
		}
	}
	return findings
}

// G-017: registered 7+ days ago, zero executions ever, service never running.
// Informational — project exists but has never been used.
func (e *Engine) ruleNeverUsed(execs []ExecutionRecord, services []ServiceRecord, projects []ProjectRecord) []*Finding {
	attempted := map[string]bool{}
	for _, ex := range execs {
		attempted[ex.Target] = true
	}
	hasRunningService := map[string]bool{}
	for _, svc := range services {
		if svc.ActualState == "running" {
			hasRunningService[svc.ProjectID] = true
		}
	}

	var findings []*Finding
	now := time.Now().UTC()
	for _, p := range projects {
		if !attempted[p.ID] && !hasRunningService[p.ID] {
			findings = append(findings, &Finding{
				RuleID:    RuleNeverUsed,
				Severity:  SeverityInfo,
				Target:    p.ID,
				Message:   fmt.Sprintf("project %q is registered but has never been executed or run — start with: engx run %s", p.ID, p.ID),
				Count:     1,
				FirstSeen: now,
				LastSeen:  now,
			})
		}
	}
	return findings
}

// G-018: no execution in 30 days, service desired=stopped.
// Informational — project was active but has gone quiet.
func (e *Engine) ruleStaleProject(execs []ExecutionRecord, services []ServiceRecord) []*Finding {
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)

	lastExec := map[string]time.Time{}
	for _, ex := range execs {
		if ex.StartedAt.After(lastExec[ex.Target]) {
			lastExec[ex.Target] = ex.StartedAt
		}
	}

	desiredStopped := map[string]bool{}
	for _, svc := range services {
		if svc.DesiredState == "stopped" {
			desiredStopped[svc.ProjectID] = true
		}
	}

	var findings []*Finding
	now := time.Now().UTC()
	seen := map[string]bool{}
	for target, last := range lastExec {
		if !seen[target] && last.Before(cutoff) && desiredStopped[target] {
			seen[target] = true
			findings = append(findings, &Finding{
				RuleID:    RuleStaleProject,
				Severity:  SeverityInfo,
				Target:    target,
				Message:   fmt.Sprintf("project %q has had no activity in 30+ days and is stopped — consider deregistering: engx project deregister %s", target, target),
				Count:     1,
				FirstSeen: last,
				LastSeen:  now,
			})
		}
	}
	return findings
}
