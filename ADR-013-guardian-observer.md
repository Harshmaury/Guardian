# ADR-013 — Guardian Observer Service

**Status:** Accepted
**Date:** 2026-03-17
**Author:** Harsh Maury
**Scope:** New observer service — Guardian
**Port:** 8085
**Depends on:** Forge Phase 4 (/history endpoint), Navigator Phase 1 (topology)

---

## Context

The platform now has four services with rich operational data. Forge logs
every execution with status (success/failure/denied). Navigator exposes the
verified project graph. Nexus emits structured events with outcome fields.

There is currently no service that monitors this data for policy violations,
anomalies, or operational warnings. A developer has no way to know if:
- Commands are being denied repeatedly (preflight failures)
- Unverified projects are being targeted
- Services are crashing in a pattern
- A project has no nexus.yaml and is blocking workflows

Guardian is the third observer service. It reads from Forge history and
Navigator topology, evaluates a set of policy rules, and exposes a findings
endpoint. It never takes action — it only reports.

---

## Decision

### 1. Guardian is strictly read-only

Never calls start/stop. Never modifies state. Only reads from:
- Forge GET /history?limit=100       (every 30s)
- Navigator GET /topology/graph      (every 30s)
- Nexus GET /events?since=<id>       (every 10s)

### 2. Policy rules (Phase 1)

Guardian evaluates these rules on every collection cycle:

| Rule ID | Name | Trigger |
|---------|------|---------|
| G-001 | Repeated denials | Same target denied 3+ times in last 10 min |
| G-002 | Unverified targets | Any execution against an unverified project |
| G-003 | High failure rate | >50% failure rate for a target in last 20 executions |
| G-004 | Service crashes | SERVICE_CRASHED event in last 5 min |
| G-005 | Unverified projects | Projects in graph with status=unverified |

### 3. What Guardian exposes

**GET /guardian/findings** — all active policy findings:
```json
{
  "findings": [
    {
      "rule_id": "G-001",
      "severity": "warning",
      "target": "unknown-project",
      "message": "project denied 4 times in last 10 minutes",
      "count": 4,
      "first_seen": "...",
      "last_seen": "..."
    }
  ],
  "summary": {"total": 1, "warnings": 1, "errors": 0},
  "evaluated_at": "..."
}
```

**GET /guardian/findings/:rule_id** — findings for one rule only.

**GET /health** — always exempt from auth.

### 4. Severity levels

- `warning` — anomaly worth investigating, not blocking
- `error` — policy violation that may indicate a real problem

### 5. Authentication

Guardian uses X-Service-Token on all outbound calls.
GET /guardian/findings requires no inbound auth — read-only.

### 6. No alerting, no actions

Guardian logs findings and exposes them via HTTP. It does not send
notifications, emails, or Slack messages in Phase 1. That is Phase 2.

---

## Implementation scope

```
guardian/
├── cmd/guardian/main.go
├── internal/
│   ├── config/env.go
│   ├── policy/
│   │   ├── model.go      — Finding, Rule, Summary types
│   │   └── engine.go     — evaluates all rules, returns findings
│   ├── collector/
│   │   ├── forge.go      — polls Forge /history
│   │   ├── navigator.go  — polls Navigator /topology/graph
│   │   └── nexus.go      — polls Nexus /events
│   └── api/
│       ├── handler/findings.go
│       └── server.go
├── go.mod
└── nexus.yaml
```

---

## Consequences

**Positive:**
- Operational awareness without manual endpoint querying
- Foundation for Phase 2 alerting (notifications, webhooks)
- Surfaces unverified projects before they cause workflow denials

**Negative:**
- New binary in startup sequence
- Polling adds minor load (negligible at local scale)

---

## Compliance

| ADR | Status |
|-----|--------|
| ADR-001 | ✅ Never maintains its own project list |
| ADR-003 | ✅ HTTP/JSON only |
| ADR-005 | ✅ Never calls start/stop |
| ADR-008 | ✅ X-Service-Token on all outbound calls |

---

## Next ADR

ADR-014 — Observer tracing service (port 8086).
ADR-015 — SSE streaming from Nexus.
ADR-016 — Platform shared types module.
