// @guardian-project: guardian
// @guardian-path: internal/policy/engine_test.go
package policy

import (
	"testing"
	"time"
)

var engine = NewEngine()

func execs(records ...ExecutionRecord) []ExecutionRecord { return records }
func nodes(ns ...TopologyNode) []TopologyNode           { return ns }
func events(es ...NexusEvent) []NexusEvent              { return es }
func services(ss ...ServiceRecord) []ServiceRecord      { return ss }
func projects(ps ...ProjectRecord) []ProjectRecord      { return ps }

func recentExec(target, status string) ExecutionRecord {
	return ExecutionRecord{Target: target, Status: status, StartedAt: time.Now().UTC()}
}

func recentEvent(typ string) NexusEvent {
	return NexusEvent{Type: typ, CreatedAt: time.Now().UTC()}
}

func assertRule(t *testing.T, report *Report, ruleID string, expectedCount int) {
	t.Helper()
	actual := 0
	for _, f := range report.Findings {
		if f.RuleID == ruleID {
			actual++
		}
	}
	if actual != expectedCount {
		t.Errorf("rule %s: expected %d finding(s), got %d", ruleID, expectedCount, actual)
	}
}

func assertNoFindings(t *testing.T, report *Report) {
	t.Helper()
	if len(report.Findings) > 0 {
		for _, f := range report.Findings {
			t.Logf("unexpected finding: [%s] %s — %s", f.RuleID, f.Target, f.Message)
		}
		t.Errorf("expected no findings, got %d", len(report.Findings))
	}
}

// ── G-001: repeated denials ───────────────────────────────────────────────────

func TestG001_TriggersOnThreeDenials(t *testing.T) {
	report := engine.Evaluate(
		execs(
			recentExec("atlas", "denied"),
			recentExec("atlas", "denied"),
			recentExec("atlas", "denied"),
		),
		nil, nil, nil, nil,
	)
	assertRule(t, report, RuleRepeatedDenials, 1)
}

func TestG001_NotTriggeredOnTwoDenials(t *testing.T) {
	report := engine.Evaluate(
		execs(
			recentExec("atlas", "denied"),
			recentExec("atlas", "denied"),
		),
		nil, nil, nil, nil,
	)
	assertRule(t, report, RuleRepeatedDenials, 0)
}

func TestG001_OldDenialsIgnored(t *testing.T) {
	old := ExecutionRecord{Target: "atlas", Status: "denied",
		StartedAt: time.Now().UTC().Add(-20 * time.Minute)}
	report := engine.Evaluate(
		execs(old, old, old),
		nil, nil, nil, nil,
	)
	assertRule(t, report, RuleRepeatedDenials, 0)
}

// ── G-003: high failure rate ──────────────────────────────────────────────────

func TestG003_TriggersOnHighFailureRate(t *testing.T) {
	report := engine.Evaluate(
		execs(
			recentExec("forge", "failure"),
			recentExec("forge", "failure"),
			recentExec("forge", "failure"),
		),
		nil, nil, nil, nil,
	)
	assertRule(t, report, RuleHighFailureRate, 1)
}

func TestG003_NotTriggeredOnLessThanThreeExecs(t *testing.T) {
	report := engine.Evaluate(
		execs(
			recentExec("forge", "failure"),
			recentExec("forge", "failure"),
		),
		nil, nil, nil, nil,
	)
	assertRule(t, report, RuleHighFailureRate, 0)
}

func TestG003_NotTriggeredOnLowFailureRate(t *testing.T) {
	report := engine.Evaluate(
		execs(
			recentExec("atlas", "success"),
			recentExec("atlas", "success"),
			recentExec("atlas", "failure"),
		),
		nil, nil, nil, nil,
	)
	assertRule(t, report, RuleHighFailureRate, 0)
}

// ── G-004: service crashes ────────────────────────────────────────────────────

func TestG004_TriggersOnRecentCrash(t *testing.T) {
	report := engine.Evaluate(nil, nil,
		events(recentEvent("SERVICE_CRASHED")),
		nil, nil,
	)
	assertRule(t, report, RuleServiceCrashes, 1)
}

func TestG004_NotTriggeredOnOldCrash(t *testing.T) {
	old := NexusEvent{Type: "SERVICE_CRASHED",
		CreatedAt: time.Now().UTC().Add(-10 * time.Minute)}
	report := engine.Evaluate(nil, nil,
		events(old),
		nil, nil,
	)
	assertRule(t, report, RuleServiceCrashes, 0)
}

// ── G-006: service maintenance ────────────────────────────────────────────────

func TestG006_TriggersOnMaintenance(t *testing.T) {
	report := engine.Evaluate(nil, nil, nil,
		services(ServiceRecord{
			ID: "atlas-daemon", ProjectID: "atlas",
			DesiredState: "running", ActualState: "maintenance",
		}),
		nil,
	)
	assertRule(t, report, RuleServiceMaintenance, 1)
}

func TestG006_NotTriggeredOnRunning(t *testing.T) {
	report := engine.Evaluate(nil, nil, nil,
		services(ServiceRecord{
			ID: "atlas-daemon", ProjectID: "atlas",
			DesiredState: "running", ActualState: "running",
		}),
		nil,
	)
	assertRule(t, report, RuleServiceMaintenance, 0)
}

func TestG006_NotTriggeredWhenDesiredStopped(t *testing.T) {
	// nexus-daemon is desired=stopped — must not trigger G-006
	report := engine.Evaluate(nil, nil, nil,
		services(ServiceRecord{
			ID: "nexus-daemon", ProjectID: "nexus",
			DesiredState: "stopped", ActualState: "maintenance",
		}),
		nil,
	)
	assertRule(t, report, RuleServiceMaintenance, 0)
}

// ── G-008: project with no service ────────────────────────────────────────────

func TestG008_TriggersOnProjectWithNoService(t *testing.T) {
	report := engine.Evaluate(nil, nil, nil,
		services(ServiceRecord{ID: "atlas-daemon", ProjectID: "atlas"}),
		projects(
			ProjectRecord{ID: "atlas"},
			ProjectRecord{ID: "orphan"},
		),
	)
	assertRule(t, report, RuleNoService, 1)
	for _, f := range report.Findings {
		if f.RuleID == RuleNoService && f.Target != "orphan" {
			t.Errorf("expected target=orphan, got %s", f.Target)
		}
	}
}

func TestG008_NotTriggeredWhenAllProjectsHaveServices(t *testing.T) {
	report := engine.Evaluate(nil, nil, nil,
		services(
			ServiceRecord{ID: "atlas-daemon", ProjectID: "atlas"},
			ServiceRecord{ID: "forge-daemon", ProjectID: "forge"},
		),
		projects(
			ProjectRecord{ID: "atlas"},
			ProjectRecord{ID: "forge"},
		),
	)
	assertRule(t, report, RuleNoService, 0)
}

// ── G-007: never built ────────────────────────────────────────────────────────

func TestG007_TriggersOnNeverSucceeded(t *testing.T) {
	report := engine.Evaluate(
		execs(recentExec("atlas", "failure")),
		nil, nil, nil,
		projects(ProjectRecord{ID: "atlas"}),
	)
	assertRule(t, report, RuleNeverBuilt, 1)
}

func TestG007_NotTriggeredOnSuccess(t *testing.T) {
	report := engine.Evaluate(
		execs(
			recentExec("atlas", "failure"),
			recentExec("atlas", "success"),
		),
		nil, nil, nil,
		projects(ProjectRecord{ID: "atlas"}),
	)
	assertRule(t, report, RuleNeverBuilt, 0)
}

// ── Clean state ───────────────────────────────────────────────────────────────

func TestEvaluate_CleanPlatformNoFindings(t *testing.T) {
	report := engine.Evaluate(
		execs(
			recentExec("atlas", "success"),
			recentExec("forge", "success"),
		),
		nodes(
			TopologyNode{ID: "atlas", Status: "verified"},
			TopologyNode{ID: "forge", Status: "verified"},
		),
		nil,
		services(
			ServiceRecord{ID: "atlas-daemon", ProjectID: "atlas", DesiredState: "running", ActualState: "running"},
			ServiceRecord{ID: "forge-daemon", ProjectID: "forge", DesiredState: "running", ActualState: "running"},
		),
		projects(
			ProjectRecord{ID: "atlas"},
			ProjectRecord{ID: "forge"},
		),
	)
	assertNoFindings(t, report)
}

// ── Report summary ────────────────────────────────────────────────────────────

func TestReport_SummaryCountsCorrectly(t *testing.T) {
	report := engine.Evaluate(
		execs(
			recentExec("a", "failure"),
			recentExec("a", "failure"),
			recentExec("a", "failure"),
		),
		nil,
		events(recentEvent("SERVICE_CRASHED")),
		services(ServiceRecord{
			ID: "b-daemon", ProjectID: "b",
			DesiredState: "running", ActualState: "maintenance",
		}),
		nil,
	)
	if report.Summary.Total == 0 {
		t.Error("expected findings in summary")
	}
	if report.Summary.Errors == 0 {
		t.Error("expected error-severity findings")
	}
}
