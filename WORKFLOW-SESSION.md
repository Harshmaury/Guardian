# WORKFLOW-SESSION.md
# Session: GD-phase1-guardian-observer
# Date: 2026-03-17

## What changed — Guardian Phase 1 (ADR-013)

New policy observer. Evaluates 5 rules (G-001 to G-005) against Forge
history, Navigator topology, and Nexus events. Exposes findings via
GET /guardian/findings.

## Setup and run

mkdir -p ~/workspace/projects/apps/guardian
cd ~/workspace/projects/apps/guardian
unzip -o /mnt/c/Users/harsh/Downloads/engx-drop/guardian-phase1-observer-20260317.zip -d .
go mod tidy && go build ./...
go install ./cmd/guardian/ && cp ~/go/bin/guardian ~/bin/guardian
GUARDIAN_SERVICE_TOKEN=7d5fcbe4-44b9-4a8f-8b79-f80925c1330e guardian &

## Verify

curl -s http://127.0.0.1:8085/health
curl -s http://127.0.0.1:8085/guardian/findings | jq '.data.summary'
curl -s http://127.0.0.1:8085/guardian/findings | jq '.data.findings[] | {rule:.rule_id, target:.target, msg:.message}'
curl -s http://127.0.0.1:8085/guardian/findings/G-005 | jq '.data.findings'

## Commit

git init && git add . && \
git commit -m "feat: guardian observer phase 1 (ADR-013)" && \
git tag v0.1.0-phase1
