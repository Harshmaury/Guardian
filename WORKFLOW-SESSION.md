# WORKFLOW SESSION — ENGX-GUARDIAN-P1-001

**Date:** 2026-03-21
**Repo:** Guardian
**Requires:** ENGX-HERALD-P1-001 applied and pushed first

## What changed

ADR-039: all raw HTTP collector calls replaced with Herald typed clients.
Raw httpClient fields removed. Anonymous struct decodes eliminated.
Schema drift on upstream API changes now caught at compile time.

## Apply

```bash
cd ~/workspace/projects/engx/services/guardian
unzip -o /mnt/c/Users/harsh/Downloads/engx-drop/ENGX-GUARDIAN-P1-001.zip -d .
go build ./...
git add internal/collector/
git commit -m "feat(guardian): ADR-039 — Herald migration for all collectors"
git push origin main
```

## Verify

```bash
go build ./...
go test ./...
engx doctor
```
