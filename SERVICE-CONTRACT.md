// @guardian-project: guardian
// @guardian-path: SERVICE-CONTRACT.md
# SERVICE-CONTRACT.md — Guardian
# @version: 0.2.0-phase2
# @updated: 2026-03-25

**Port:** 8085 · **Domain:** Observer (read-only)

---

## Code

```
internal/collector/nexus.go     polls GET /events?since=<id>&limit=100
internal/collector/forge.go     polls GET /history?limit=100
internal/collector/navigator.go polls GET /topology/graph
internal/policy/engine.go       Evaluate(PolicyInput) → Report
internal/policy/model.go        Finding, Report structs
internal/api/handler/findings.go  GET /guardian/findings
```

---

## Contract

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | none | Liveness |
| GET | `/guardian/findings` | token | `GuardianReportDTO` — findings + summary + evaluated_at |
| GET | `/guardian/findings/:rule_id` | token | Filtered to one rule |

Response type: `accord.GuardianReportDTO`.

**Active rules:**

| Rule | Condition | Severity |
|------|-----------|----------|
| G-001 | Same target denied 3+ times in 10 min | warning |
| G-002 | Forge executed against unverified Atlas project | warning |
| G-003 | >50% failure rate for target in 20 min | error |
| G-004 | `SERVICE_CRASHED` in last 5 min | error |
| G-005 | Topology node `status=unverified` | warning |

---

## Control

`PolicyInput` snapshot assembled from all three collectors before `Evaluate()` — findings never span mixed time-points. Deterministic: same input → same output. Per-cycle trace ID: `gd-<hex>`. One full pass before HTTP server starts.

---

## Context

Audit layer only. Never blocks execution. Never calls write endpoints. Findings are point-in-time and lost on restart.
