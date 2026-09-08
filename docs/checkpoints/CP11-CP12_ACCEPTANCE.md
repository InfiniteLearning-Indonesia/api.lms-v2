# CP11–CP12 local implementation contract — 8 September 2026

User continuation selects the proposed server bridge (master plan §16.2): hardened legacy remains global identity owner. No legacy/shared write, deploy, commit or push. CP11 and CP12 are separately tested local slices, not completion of all M04/M05 criteria.

CP11 frozen scope: pinned EdDSA JWT bridge verifier (issuer/audience/purpose/expiry/nonce/version), reviewed issuer+subject mapping (no email merge), DB single-use challenge/JTI and fresh Go cookie session issuance; rotation/logout/revoke-all with retained rows, monotonic signed account projection and bounded freshness, local site-admin demotion invariant. Global profile/password/invite/reset are explicitly owner-only/unavailable, not independently implemented in Go. Local simulated issuer produces assertions, never inserts authenticated Go sessions.

CP12 frozen scope: centralized capability table, Teacher manages Students only (never staff grants/targets), Class close/reopen, enrollment suspend/reactivate without ending the participation episode, last-teacher guard, retained transition evidence, and shared/exclusive DB authorization fence against in-flight Go Class mutations. Grouping/personal mentor/invited/retake/content-ready publish remain full M05 dependencies, not hidden completed features.

Quantitative limits: JWT ≤8 KiB, EdDSA only, pinned kid, assertion ≤60 seconds, no leeway, required iat/nbf/exp; subject explicit reviewed mapping; session idle window 15 minutes with one-hour absolute limit, rotated explicitly; account freshness expires with signed assertion (≤60 seconds). No claim of instantaneous remote legacy revoke: only committed local revocation is fenced. Authority unavailable/no fresh assertion fails closed after that bound. No network request while holding domain transaction locks.

Separate identity writer and Class runtime roles; no DDL/session/account writes through Class pool. Existing class-slice-1 schema/checksum remains unchanged. Each additive installment has its own fresh SQL/custom dump, isolated restore/reconciliation, receipt, checksum and advisory lock. No DELETE/DROP/TRUNCATE; all runs retained.

## Deliverable/QA mapping (all required within the local slice)

| D | Required evidence |
|---|---|
| CP11-D01 | Verifier unit rejection matrix + real verified exchange API; wrong issuer/audience/signature/key/purpose/time/nonce/unknown subject/email-only denied |
| CP11-D02 | Real session API issue/rotate/logout/revoke-all and cookie/CSRF/header denial; DB retained credential hashes/no raw tokens; concurrent consume/rotate only one succeeds |
| CP11-D03 | Signed projection monotonic version/disable/freshness; owner-only commands denied; local site-admin last-demotion serialized; global owner not claimed |
| CP11-D04 | Identity transaction rollback, permission denial, backup/restore + current Go/Playwright/default regression |
| CP12-D01 | Capability unit matrix + real multi-Class/role target-denial; Teacher Student-management positive and staff-management negative |
| CP12-D02 | Suspend/resume/close/reopen/version/idempotency/last-teacher API+DB history assertions |
| CP12-D03 | Barrier-based revoke-versus-Class-write and role change concurrency on actual PostgreSQL; post-revoke requests denied |
| CP12-D04 | CP11 + existing CP08–CP10 regression, backup before additive installment, source-bound evidence |

Exact executable IDs will live in `qa/manifests/CP11-CP12.json`; any missing/skipped/failed expected case blocks its slice. Report pass rate, code coverage by measured package and D coverage separately. Changes to frozen criteria require a visible update, not silently dropping failing scenarios.

Final verification: [8 September results](2026-09-08-cp11-cp12.md), CP11 4/4 and CP12 4/4 **VERIFIED_LOCAL**. Manifest expanded with CP11-replay-rate and CP12-prerequisites to strengthen the same scope; no required scenario removed. `qa.checkpoint_report` derives delivery status from current exact-case Go/Playwright/M00/M03/Linux evidence and unions identical coverage blocks. Full M04/M05 completion and external gates below remain unclaimed.

External gates: real hardened legacy issuer/provisioning/reset/last-global-admin, provider key management/rotation deployment, browser frontend compatibility, full capability/grouping/retake, production/staging identity and secure export. Lab HTTPS uses an ephemeral certificate; accepting that test certificate is not evidence of production TLS configuration. `cmd/api` remains non-deployed bootstrap; opt-in lifecycle lab only.
