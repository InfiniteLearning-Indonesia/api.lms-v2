# CP01–CP05 — bounded implementation session

> Update 7 September: CP05 local-cluster permission was subsequently granted and its real synthetic integration passed. See [CP05 closure](2026-09-07-cp05-closure.md). Pending-approval/NOT_RUN statements below describe this document's earlier handoff, not the latest CP05 status.

User approved exactly five checkpoints across M00/M01/M03. Backend Go only; Python orchestrates QA. These CP IDs are session work slices, not replacements for M00–M19 or new acceptance denominators.

## Outcomes and remaining acceptance

| Checkpoint | Implemented deliverable | Verification / remaining boundary |
|---|---|---|
| CP01 — Evidence reconciliation | `qa/observations.py`, explicit `--observation` + `--expected-target-fingerprint`; validates source, target, timestamp (24h), digest, TLS, read-only and scope. Separate generated projection; no promotion of full module. M00 blockers split. | Offline positive/negative QA PASS. Historical receipts without the new digest stay historical/rejected. Local checksum is integrity evidence, not a signed operator approval. |
| CP02 — Metadata/access inventory | Go `dependencies.go`: non-system namespaces/relations, trigger metadata, routine flags/hashes and effective API-role table privileges. [Access model](../baseline/ACCESS_MODEL.md). | Real staging SELECT succeeded with TLS/read-only. Full schema semantics, job definitions, storage, actual production and API exposure config remain pending. No role/grant/RLS modifications. |
| CP03 — Offline backup preflight | Go `cmd/backup-plan`, `internal/migration/backup`: strict JSON, scope/identity/path/capacity declarations, no-overwrite checks, local artifact digest/freshness verifier. | Unit positive/negative tests PASS. Planner never invokes pg_dump/restore or authorizes execution. Declared free space/ACL/approval flags are NOT actual verified operating-system checks. Production orchestration, encryption/key recovery, complete scope and real backup remain pending. |
| CP04 — Artifact/Linux QA | Static Linux amd64 build twice with byte equality; allowlisted tar artifact and scratch Dockerfile UID/GID 65532. Linux QA wrapper for race + non-root loopback smoke; CI build step added without publish/deploy. | Build/static checks PASS. See final evidence below for Linux run; container execution and remote CI remain separate NOT_RUN gates. Artifact does not enable staging/production runtime. |
| CP05 — Transaction and isolated harness | Go `transaction.Within` with bounded rollback, cancellation/panic handling and no automatic commit retry. Tagged pgxpool test for atomic synthetic domain/audit/outbox, exhaustion and cancellation. Opt-in fresh PostgreSQL lab with backups and isolated restore proof before fixture DDL. | Unit/runner boundary tests PASS. Real lab execution requires explicit new-cluster approval and is not inferred from tool installation. No business repository, durable audit/outbox worker or production runtime pool is claimed complete. |

Canonical M00/M01/M03 remain **0/5 full deliverables each**, total M00–M18 **0/97**. This is not zero work: the table above records concrete partial scope. No denominator changes and no fabricated % effort estimate.

CP05 execution gate at handoff: approval for creating the new local PostgreSQL cluster was requested during the session and no answer was received. Harness and tagged tests were compiled, but **no synthetic cluster, database backup or real transaction integration test was executed**. The compile-only command reported `[no tests to run]`; this is not an integration PASS. Resume this existing CP05 scope after approval before starting CP06.

## Final cross-platform evidence

Canonical current executable source hash: `1540f4c2f4853b0e7e9d4548dd3bb454fcb4f459b0508ceff7f1e12e7af507ab` (Windows and Linux agree).

| Final cumulative QA | Run | Verified result |
|---|---|---|
| M00 | `20260906T134852Z-M00-071e1c42dafb` | **19/19** checks PASS; 15 Python cases + 4 aggregate Go checks; 17 Go cases |
| M03 | `20260906T135016Z-M03-2e2d51477583` | **22/22** checks PASS; 18 Python cases + 4 aggregate Go checks; 17 Go cases |

Both final runs have `publication.json`, the same current source hash, no missing/unexpected cases, and separate staging observation `OBSERVED_NOT_APPROVED`. **Go statement coverage: 70.7% (previous 67.4%)**. Default full-package status remains BLOCKED, not a test failure. No synthetic PostgreSQL integration coverage is included. Markdown validation: 39 files, zero broken local links, unbalanced fences or replacement characters.

- Linux QA: `20260906T134601Z-linux-qa-6c41ab86c121`, **PASS**, 17 registered Go tests under race detector; actual loopback API runtime real/effective/saved/filesystem UIDs all **65532**. This is non-root process proof, not container execution or remote CI.
- Final static artifact: `20260906T134840Z-release-ed9e2695cbef`, **PASS_BUILD_ONLY**; two builds byte-identical, static ELF Linux/amd64. Binary SHA256 `c7d444880f1e6b4d31ea3f9b121186a9b7eccbd7a4c2805b5ac3067f294c3682`. Tar contains only `api` and `Dockerfile`.
- Final staging metadata: `20260906T134834Z-staging-inspect-5264d297524d`, verified TLS/read-only, exact current source; separately reconciled into final M00/M03 QA, still **OBSERVED_NOT_APPROVED**.
- `go mod verify`: all modules verified. Tagged transaction integration compiled successfully; actual DB tests NOT_RUN pending local-cluster approval. Go dependency cache integrity is not a vulnerability scan.

Final regression exposed a cross-platform reporting bug: sorting Path objects uses different ordering on Windows/Linux. `source_fingerprint` now sorts explicit relative POSIX strings, and a mixed-case filename fixture guards the behavior. Earlier Linux run `20260906T134016Z-linux-qa-8bf7c6b4b3e0` passed tests but used the older platform-dependent hash; corrected Linux proof above supersedes it. Historical receipts were not rewritten or falsely relabeled current.

## QA chronology (retained, not overwritten)

| Checkpoint | Run | Result |
|---|---|---|
| CP01 first attempt | Runner exception in retained M00 run before final summary | Local variable shadowing made inventory result replace observation path; fixed and rerun. No DB action. |
| CP01 corrected | `20260906T125329Z-M00-114af73d0575` | 17/17 bootstrap checks PASS, full module BLOCKED |
| CP02 | `20260906T125526Z-M00-16c943dd4e1e` | 17/17 bootstrap checks PASS, full module BLOCKED |
| CP02 real staging | `20260906T125600Z-staging-inspect-dd22701de550` | TLS/read-only/static metadata success; not migration approval |
| CP03 | `20260906T125805Z-M03-2814db5152b1` | 20/20 bootstrap checks PASS, 15 Go cases; full module BLOCKED |
| CP04 artifact | `20260906T130107Z-release-98fe01d9e65d` | Static Linux artifact, deterministic repeated build PASS; container not run |
| CP04 regression | `20260906T130208Z-M03-7b2439001bfd` | 21/21 bootstrap checks PASS, 15 Go cases; full module BLOCKED |
| CP05 regression | `20260906T130835Z-M03-233775abc5ed` | 22/22 bootstrap checks PASS, 17 Go cases, Go statement coverage 70.7%; full module BLOCKED |

Rows above are checkpoint-specific source hashes; later implementation makes earlier source-specific evidence historical. Latest cumulative evidence is recorded below. Go/Python shared cases are not unique across runs; pass counts must not be summed. Tagged DB tests are deliberately excluded from automatic local-synthetic QA and fail rather than skip if invoked without the isolated harness.

## New security observation — requires owner review

Staging metadata reported 8 namespaces, 60 relations (including non-table relation kinds), 205 triggers (including internal FK triggers), 97 routines and 40 public table/API-role observations. There were no `cron`/`pgmq` relation entries; this does NOT prove no external jobs exist.

Both `anon` and `authenticated` have effective SELECT on all 20 public tables. On `users` and `session` specifically, schema USAGE plus SELECT/INSERT/UPDATE/DELETE/TRUNCATE are true. Prior same-target catalog shows RLS=false. **This is a high-priority configuration risk; public endpoint reachability/exploitation has not been tested.** A database TRUNCATE grant is not a claim that PostgREST exposes a TRUNCATE endpoint. Verify the project's exposed schemas/API configuration and legacy access dependencies before remediation. Do not enable RLS/revoke grants indiscriminately or assume the rewrite secures live.

Source: private `.qa-artifacts/20260906T125600Z-staging-inspect-dd22701de550/summary.json`; query `internal/platform/dbinspect/dependencies.go`. No student/session row values, raw function bodies, trigger arguments, job commands or secrets were fetched. See [Supabase API security](https://supabase.com/docs/guides/api/securing-your-api) for grants vs RLS.

## Safety and limits

- No production access/export/copy, shared DB DML/DDL, role changes, storage change, frontend/legacy patch, commit/push or deployment.
- Existing user deletions preserved. New code remains in backend-go, plus the existing root CI workflow integration.
- Staging inspector remains static metadata only. pgxpool in the tagged synthetic test is not wired to API runtime.
- Production backup/migration status remains NOT_EXECUTED. Synthetic fixture backups, if authorized/executed, must be labeled separately.
- Scratch artifact deliberately contains no CA bundle: current API has no outbound/DB function. Runtime TLS trust packaging must be designed before enabling those capabilities, not silently bypassed.
- Skills Supabase/Postgres kept verified TLS, catalog-only scope and role separation in place. No schema mutation used to diagnose access.

## Commands / operational distinction

```powershell
# Offline default QA: no shared DB connection
.\.venv\Scripts\python.exe -m qa.run --package M00 --workspace D:\Projects\lms-v2 --update-docs
.\.venv\Scripts\python.exe -m qa.run --package M03 --update-docs
# Static artifact only, no Docker required
.\.venv\Scripts\python.exe -m qa.release
# Only after explicit NEW local synthetic cluster approval
.\.venv\Scripts\python.exe -m qa.postgres_lab --create-local-synthetic-cluster --pg-bin 'C:\Program Files\PostgreSQL\18\bin'
```

Lab creates unique retained `.qa-artifacts/<run>/postgres`, binds numeric loopback with generated SCRAM password, creates only new synthetic databases, backups prestate/fixture and restores into new proof databases without `--clean`. It stops only its own server and retains cluster/backup files; no cleanup deletion. Loopback-only synthetic transport uses SSL disabled explicitly and is not a permitted staging/production configuration. Actual backend TLS remains verified. PostgreSQL major-version differences between local 18 and staging 17 must remain visible.

PostgreSQL archive/SQL rendering design follows [pg_dump](https://www.postgresql.org/docs/17/app-pgdump.html); a restored dump can execute source-defined code, so shared-data restore requires scope review and isolation, not just archive readability. Linux toolchain downloaded into project cache and verified against [official Go download checksums](https://go.dev/dl/?mode=json), no global installation.

## Next five checkpoints — BRIEF ONLY

See [CP06–CP10](NEXT_FIVE_CHECKPOINTS.md). Nothing from those checkpoints is implemented as an additional scope in this session.
