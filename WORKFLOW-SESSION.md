# WORKFLOW-SESSION.md
# Session: GD-fix-canon-migration
# Date: 2026-03-19

## What changed

Canon migration (ADR-016). Replaced 3 raw "X-Service-Token" string
literals in NexusCollector with canon.ServiceTokenHeader.

## Modified files
- internal/collector/nexus.go  — canon import added, 3 raw strings replaced

## Apply

cd ~/workspace/projects/apps/guardian && \
unzip -o /mnt/c/Users/harsh/Downloads/engx-drop/guardian-fix-canon-20260319.zip -d . && \
go build ./...

## Verify

grep -c 'canon.ServiceTokenHeader' internal/collector/nexus.go
# Expected: 3

grep '"X-Service-Token"' internal/collector/nexus.go
# Expected: (no output)

## Commit

git add internal/collector/nexus.go WORKFLOW-SESSION.md && \
git commit -m "fix: Canon migration — replace raw X-Service-Token in NexusCollector (ADR-016)" && \
git push origin main
