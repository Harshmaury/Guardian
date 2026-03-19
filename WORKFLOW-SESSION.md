# WORKFLOW-SESSION.md
# Session: GD-phase2-trace-warnings
# Date: 2026-03-19

## What changed — Guardian Phase 2 (audit #3, #4)

Per-cycle trace ID: every evaluation cycle now generates a unique gd-<hex>
trace ID propagated to all upstream calls via X-Trace-ID. Collectors now
log WARNING on upstream failure instead of returning nil silently.

## New files
- (none)

## Modified files
- cmd/guardian/main.go              — guardianVersion 0.2.0, newCycleTraceID(),
                                      logger injected into all collectors,
                                      traceID passed to Collect calls
- internal/collector/forge.go      — *log.Logger field, WARNING on failure
- internal/collector/navigator.go  — *log.Logger field, WARNING on failure
- internal/collector/nexus.go      — *log.Logger field, traceID on all 3 methods,
                                      WARNING on failure, unified get() helper

## Apply

cd ~/workspace/projects/apps/guardian && \
unzip -o /mnt/c/Users/harsh/Downloads/engx-drop/guardian-phase2-trace-warnings-20260319.zip -d . && \
go build ./...

## Verify

go build ./...
GUARDIAN_SERVICE_TOKEN=<token> ./guardian &
# Should see in logs: "evaluated trace=gd-<hex> — N finding(s)"
# Kill a service, wait 30s: should see "WARNING: nexus collector (events) unreachable: ..."

## Commit

git add \
  cmd/guardian/main.go \
  internal/collector/forge.go \
  internal/collector/navigator.go \
  internal/collector/nexus.go \
  WORKFLOW-SESSION.md && \
git commit -m "feat(phase2): per-cycle trace ID + WARNING logs on upstream failure (audit #3, #4)" && \
git tag v0.2.0-phase2 && \
git push origin main --tags
