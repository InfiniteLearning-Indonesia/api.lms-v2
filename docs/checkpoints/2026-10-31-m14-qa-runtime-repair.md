# M14-CP01/02 — coverage instrumentation and shutdown repair

31 October 2026. User requested continuation after failed run `20261031T103230Z-postgres-lab-e14518526f85`. The scope remains verification of CP01/02 before CP03. Neither checkpoint is closed by this repair.

## Two confirmed failures

1. The run summary reports `m14cp01 reporting API coverage missing`. Raw files show **2/2 CP01 Go cases and 3/3 CP01 Playwright API cases PASS**. The actual `go build -coverpkg=...` command for `identity_binary` omitted `./internal/reporting`, so the API tests could exercise reporting successfully without recording that package's coverage. The guard correctly prevented acceptance without required coverage evidence. CP02 was not reached.
2. Final status was `FAIL_STOP_REQUIRES_OPERATOR` because `pg_ctl -m fast -w -t 15 stop` timed out. `postgres.log` records fast shutdown request at `17:55:32.771 +07` and `database system is shut down` at `17:55:51.107 +07`. At inspection the retained cluster's `postmaster.pid` was absent. This evidence supports completed delayed shutdown; no force stop, process kill, data deletion or database command was performed during diagnosis. The old run remains FAIL, not retroactively PASS.

## Changes

- Added `./internal/reporting` to the actual identity-lab binary's explicit coverage package list. Retained the mandatory reporting coverage assertion and exact API case checks.
- Raised bounded fast-stop wait to 45 seconds with a 55-second subprocess timeout. No immediate shutdown or force kill; real shutdown failure still blocks the run.
- Added supplemental preflight regressions `M14-CP02-instrumentation-regression` and `Q00-postgres-stop-budget`. They inspect the actual build/stop calls to prevent the observed omissions. These supplement the frozen CP manifests; they do not replace API/DB tests or alter the denominator of the required CP cases.
- Existing `M13-CP04-runner-order` regression remains enabled and locally passing.

## Acceptance still pending

Local run `20261031T110545Z-m14-runtime-repair-b962ff31` is **PASS_LOCAL_RUNTIME_CHECKS_ONLY**, source `f4819963e757a6afb285a11403459d21c55fa9d80d819803d2ee6eaca33a8303`, unchanged during checks. All six checks passed: three supplemental/existing runner regressions via pytest, manifest review, full Go suite, Go vet, actual identity-lab build extracted from the runner AST, and compiled binary symbol inspection. Two reporting coverage symbols were present. Binary SHA-256: `81fc065245e1e0f1a362114921b1810a1a8fdf1536c54359b6750d8ac9e2d8b1`. The binary was not started; no database was contacted. This is build/instrumentation evidence, not measured reporting API hit coverage or a full CP PASS.

Run fresh `--reporting-contract-journey` in ordinary Windows PowerShell after local build verification. Review current source fingerprint, reporting instrumentation hit counts, exact Go/API cases for both CPs, all dependency regressions, backup/restore hashes, reconciliation and final stop result. Keep old run evidence unchanged.

CP01/02 remain IMPLEMENTED_UNVERIFIED (M14 0/4 local CPs; canonical 0/5). Five passing CP01 case results from a failed old run are diagnostic evidence, not current-source closure. CP03 is still gated and has not started. This change adds no schema or shared database access, and no commit/push/deployment.
