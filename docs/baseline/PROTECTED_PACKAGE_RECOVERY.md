# Protected encrypted output and authenticated package consumption

> **Dependency accepted, continuation pending:** [Seven-case owner result](../checkpoints/2026-11-17-protected-package-accepted.md) accepted on source `126c217…`. [Local seal/recovery extension](LOCAL_RECOVERY_RUNBOOK.md) adds explicit plaintext-recovery behavior and two native cases (nine total), plus a separate ten-case DB recovery profile. [Current result](../checkpoints/2026-11-17-local-recovery.md) remains native/DB pending; historical seven-case acceptance is not a current full-suite PASS.

17 November 2026. Supports **M01-CP02 / DLV-03–04**. [Frozen scope](../checkpoints/2026-11-17-protected-package-scope.md); actual execution results will be recorded in its companion checkpoint. This is local filesystem/crypto tooling, not source export, restore execution or real-data authority.

## Producer: exclusive protected ciphertext

[SealProtectedAgeFile](../../internal/migration/backup/protected_age_file.go) accepts a reader and `ProtectedAgeRequest`: existing output directory, `.age` basename, stream request, expected plaintext SHA-256, identity path and custody reference. It does not open a source database or certify where reader bytes originated.

Before payload write it requires actual capacity (cipher budget + 8 GiB reserve), valid bounded/deadline inputs, pinned age tool, private output directory, private key directory/file, single-linked key, and key outside the artifact directory. Different local Windows volumes are supported; absolute clean paths, no UNC/link/reparse traversal, and actual protection checks still apply. A custody reference is not independent key custody verification.

The directory is opened with `os.OpenRoot`. Output uses anchored **O_CREATE | O_EXCL | O_RDWR**, owner-only mode on Linux and inherited DACL checked on Windows before bytes. Existing files, symlinks, overwrite/retry and unsafe key paths fail. Directory/file identity and protection are checked around operations; files with multiple hard links fail. No existing ACL, mode, owner or inheritance is repaired automatically.

It streams standard age framing, checks expected plaintext digest/size, syncs ciphertext, seeks and verifies age authentication to EOF, rechecks identity/protection and closes before returning a success report. It writes **no plaintext file**. Error/cancellation/wrong key/source mismatch leaves any created partial ciphertext intact and returns **no success receipt**; `.age` extension alone is not a verified artifact. No rename, promotion, overwrite or deletion occurs.

Success is `PASS_PROTECTED_ENCRYPTED_FILE_VERIFIED_ONLY`; local protection/authentication true, independent custody/actual restore/authority false. Parent directories and local host must be controlled by a trusted operator; this is not a defense against a malicious administrator or continuously adversarial path/ACL replacement.

Producer/readback currently needs access to the decryption identity. **This single-process proof is not an independent-custody production exporter.** Public-recipient source export and independent recovery/operator custody remain operational design/gates, rather than booleans the caller may set true.

## Native protection policy

- Windows: read owner/DACL from actual handles with [GetSecurityInfo](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-getsecurityinfo); no SetSecurityInfo or privilege enablement. Require owner = current process user, an owner full-access grant, and ordinary allow ACEs only for that user/LocalSystem. Null/empty/broad/unknown/object/callback/deny/inheritance-only grants fail closed. This deliberately narrow policy may reject safe but unsupported enterprise ACL layouts; do not broaden them silently to get PASS.
- Linux: handle owner UID must equal effective UID, group/other mode bits zero; regular key/output files require one hard link. Unsupported hosts fail closed.
- Native QA creates new synthetic directories with restrictive ACL **at initial creation**, using [SDDL conversion](https://learn.microsoft.com/en-us/windows/win32/api/sddl/nf-sddl-convertstringsecuritydescriptortosecuritydescriptorw) and CreateDirectory. It never changes an existing directory ACL or grants the sandbox extra access. Go test TempDir cleanup concerns only its own newly-generated synthetic fixtures, not existing data or retained artifacts.
- Identity support remains one bounded unencrypted X25519 age key. Key reads now use anchored bounded I/O (16 KiB + sentinel), rather than an unbounded `ReadFile` after a size observation. Key values are not logged, returned or included in source backup.

## Consumer: explicit authenticated mode

[baseline-package](../../cmd/baseline-package/main.go) now has **`--authenticate-only`**, separate from `--verify-only` and `--plan-restore`. It accepts one strict bounded JSON object on stdin; no `.env`, DSN, credentials, server connection, shell or SQL execution. Invalid/blocked inputs exit 2. Exit 0 means only the report's stated offline scope.

Input fields:

| Field | Contract |
|---|---|
| `version` | 1 |
| `package` | Existing complete `PackageRequest`/snapshot/catalog/file inventory contract; input artifact directory has no extra manifest/key/report file |
| `tool` | Absolute age tool path, SHA-256 and pinned module checksum; actual Go build provenance verified before execution |
| `identity_path` | Private, single-linked identity outside artifact directory; key content not inline JSON |
| `artifacts` | Exact one receipt per encrypted `db_custom`, `db_sql`, and optional `object_bundle`; no missing/duplicate/relabelled receipt |

DB/custom/SQL/object payloads must all be age encrypted for this mode; metadata stays bounded plaintext within the private directory, with hashes and actual file protection checked. Plaintext DB artifacts may still pass the lower integrity-only mode but cannot be promoted to authenticated-package status.

Before encrypting an artifact, use [PackageAgeBinding](../../internal/migration/backup/authenticated_package.go) to bind the inner encrypted manifest to intake run/source declaration, full snapshot declaration, target, sorted schemas, **role + basename + plaintext size/hash**. Ciphertext hashes are deliberately excluded from this inner binding to avoid circular dependencies. Snapshot/source fields must be frozen before sealing; changing them requires new reviewed input/receipts, not retimestamping old evidence.

Each ciphertext is opened anchored, protection/hardlink checked and consumed by actual age authentication/EOF verification. Receipts must match declared ciphertext size/hash and recomputed inner binding. Updating an outer hash or receipt success flag cannot substitute for decryption. Inventory hashes/directory identity are rechecked after crypto; only then can whole-package authentication become true. This is not an atomic snapshot against a malicious privileged local writer; operator-controlled immutable input remains required.

Success: **`OFFLINE_PACKAGE_AUTHENTICATED_NOT_AUTHORIZED`**, with local protection and encrypted-archive authentication true. Signed source provenance, consistent production snapshot, catalog/object completeness, independent custody/recovery, actual SQL/archive validity, restored objects/database and operational authority remain **unverified**. Public-key encryption can authenticate ciphertext integrity for the recipient; it does not prove who supplied or approved the source data.

Existing `--verify-only` and `--plan-restore` keep their old lower proof flags/status. The static planner does not gain restore execution or actual-restore proof from the new CLI option.

## QA contract and one-command owner runner

[M01-PROTECTED-PACKAGE.json](../../qa/manifests/M01-PROTECTED-PACKAGE.json) adds four default safety tests, three actual pinned-tool tests and one ops CLI refusal case. [M01-AGE-STREAM.json](../../qa/manifests/M01-AGE-STREAM.json) retains the previous four mandatory crypto cases and now requires **all seven**. Missing/fail/skip/duplicate/unexpected/stale cases cannot pass. No canonical denominator or external gate is reduced.

Use an ordinary terminal of the directory/key owner, without editing source during execution:

```powershell
D:\Projects\lms-v2\backend-v3\backend-go\qa\RUN_PROTECTED_PACKAGE_QA.cmd
```

This wrapper uses cached pinned tools (`--no-download`), storage lock/admission, reopened/hash-verified source backup, fresh baseline-package build, exact seven actual cases (including actual compiled CLI positive verification), exact default Go regression and vet. Initial storage measurement now prints progress before scanning. No PostgreSQL cluster, DB operation, retention cleanup, ACL relaxation, system installation, commit or push.

If verified cache is absent, stop and review acquisition through the existing age runner; do not place arbitrary binaries or forge install proof. New synthetic positive fixtures intentionally require restrictive ACL access. Windows restricted-token failure is a real FAIL/pending-native-evidence state, not a reason to grant sandbox access, skip tests, remove protection assertions or report PASS from compile-only evidence.

The positive tests exercise protected exclusive write/readback, wrong-key/partial-input preservation, public-directory refusal and key hardlink refusal; package tests cover all three payload roles plus CLI, receipt reordering, incomplete coverage, forged inner binding, wrong identity and corruption with updated outer hashes. Synthetic `PGDMP`/SQL payloads are header fixtures, **not valid restored PostgreSQL backups**.

## Limits before real baseline

1. Require actual owner-terminal positive results on the current tree; review evidence rather than accepting a prepared runner as successful QA.
2. New private directories at test creation are allowed; automatically repairing existing ACLs/ownership is not implemented or authorized here.
3. Freshness remains the existing 24-hour binding/snapshot/intake window. Long-term historical archive recovery needs a separately reviewed recovery contract; never rewrite timestamps/receipts to force a fresh-package PASS.
4. Actual isolated SQL/custom/object restore and reconciliation remain the next deliverable after this slice passes; nothing here extracts plaintext or executes restore.
5. Snapshot acquisition, independent key custody, source/target identity, real mapping, current full Playwright/remote CI and staging runtime composition remain separate. No switch to live/staging is granted by a crypto report.
