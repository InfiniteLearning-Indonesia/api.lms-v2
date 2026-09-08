# CP08–CP10 — frozen local slice, 7 September 2026

Authorized after clarification: `backend/.env` and backend-go `.env.staging` both point to staging. Production reference remains unavailable. No shared connection, copy, schema/grant change, deployment or automatic push in this slice. Existing CP07 evidence remains historical; latest user classification supersedes the temporary production interpretation.

Backend Go; Python/Playwright QA; no frontend. Frozen slice criteria below do not replace full M01/M03/M04/M05 acceptance.

| Checkpoint | Local acceptance | Residual operational gates |
|---|---|---|
| CP08 | AES-GCM artifact authentication bound to run/target/time, wrong-key/tamper/stale/wrong-target rejection; actual capacity/host protection checks fail closed; real fixture dump recovery/restore; retained original artifacts | Production identity/export, storage bytes, protected real-data destination, independent key custody/recovery approval |
| CP09 | Restricted runtime pool without DDL; durable Class+audit+outbox+idempotency in one transaction; guarded additive synthetic schema installation with backup receipt/lock/checksum; outbox lease/ack fencing and retry semantics | Shared role provisioning, production pool/TLS integration, general-purpose migration tooling and real provider delivery |
| CP10 | Go HTTP→opaque session→database-backed actor/Class policy→Class/participant commands; multiple Teachers and Students; enrollment and role separate; publish/archive/end history; mass assignment/foreign Class/disabled/revoked/retry/conflict/concurrency tests | Real identity issuer/login/bridge, browser cookie/CSRF compatibility, complete episode/grouping workflows and frontend |

QA contracts: default Go registry explicitly expanded; integration expected names in the lab are exact; Playwright cases have explicit IDs in `qa/manifests/CP08-CP10.json`. Missing, skipped or failed cases block the slice. No claim of full module 100% from slice results. All DB writes run only after a fresh local fixture backup/restore receipt. Failed fixtures and archives retained; stop only processes created by the lab.

Identity boundary: test identities/sessions are provisioned by the isolated operator fixture, never by an HTTP login bypass. The business handler consumes hashed opaque bearer sessions; cookie authentication is not enabled. `cmd/api` stays bootstrap-only; the opt-in lab entry point cannot accept an arbitrary/shared DSN or dotenv. Provider login/issuance remains disabled until reviewed M04 authority.
