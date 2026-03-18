# SERVICE-CONTRACT.md — Guardian

**Service:** guardian
**Domain:** Observer
**Port:** 8085
**ADRs:** ADR-013 (capability), ADR-020 (governance)
**Version:** 0.1.0-phase1
**Updated:** 2026-03-18

---

## Role

Read-only policy observer. Evaluates deterministic rules against collected
platform data and produces findings. Guardian has no execution authority.

---

## Inputs

- `Forge GET /history?limit=100` — execution records for rule evaluation
- `Navigator GET /topology/graph` — workspace topology nodes for status checks
- `Nexus GET /events?since=<id>&limit=100` — recent platform events

All inputs are read-only HTTP GET calls. Guardian never writes to any upstream.

---

## Outputs

- `GET /health` — `{"ok":true,"status":"healthy","service":"guardian"}`
- `GET /guardian/findings` — full policy findings report
- `GET /guardian/findings/:rule_id` — findings filtered to one rule

Response shape for findings:
```json
{
  "ok": true,
  "data": {
    "findings": [...],
    "summary": {"total": 0, "warnings": 0, "errors": 0},
    "evaluated_at": "..."
  }
}
```

---

## Dependencies

| Service   | Used for                        | Auth required |
|-----------|---------------------------------|---------------|
| Forge     | Execution history               | X-Service-Token |
| Navigator | Workspace topology nodes        | None (ADR-012) |
| Nexus     | Recent platform events          | X-Service-Token |

Guardian does NOT depend on Atlas or Sentinel.

---

## Policy rules

| Rule  | Trigger                                          | Severity |
|-------|--------------------------------------------------|----------|
| G-001 | Same target denied 3+ times in last 10 minutes  | warning  |
| G-002 | Forge executed against unverified Atlas project  | warning  |
| G-003 | >50% failure rate for target in last 20 minutes | error    |
| G-004 | SERVICE_CRASHED events in last 5 minutes        | error    |
| G-005 | Topology node with unverified status             | warning  |

---

## Guarantees

- Evaluation is performed against a consistent point-in-time `PolicyInput`
  snapshot — all three collector results are assembled before `Evaluate()` runs.
- Graceful degradation — if any upstream is unreachable, Guardian serves
  stale data and logs a WARNING. It never crashes on upstream failure.
- One full collection pass completes before the HTTP server starts (ADR-020 Rule 6).
- Findings are deterministic — same input always produces same output.
- Each collection cycle carries a unique `gd-<hex>` trace ID on all outbound calls.

---

## Non-Responsibilities

- **Guardian never blocks execution.** It cannot prevent a command from running.
- **Guardian never calls** `POST /projects/:id/start` or `/stop` on Nexus.
- **Guardian never calls** `POST /commands` on Forge.
- **Guardian never writes** to any platform database.
- **Guardian never triggers** workflows or automation.
- **Guardian is not a control layer.** It is an audit layer.
- Guardian does not own project state, service state, or execution authority.

---

## Data Authority

**Derived, non-authoritative.**

Guardian findings are computed from data owned by other services:
- Execution truth → Forge owns it
- Topology truth → Atlas owns it (Navigator derives from Atlas)
- Event truth → Nexus owns it

Guardian findings reflect a point-in-time evaluation. They are not
persistent truth — they change on the next evaluation cycle.

---

## Concurrency Model

- `ReportStore` protected by `sync.RWMutex`. `Set()` takes write lock,
  `Get()` takes read lock.
- Single polling goroutine owns all collection and store writes.
- HTTP handlers are read-only — they call `Get()` only.
- `PolicyInput` assembled from all collector buffers before `Evaluate()` —
  findings never span mixed time-points.
