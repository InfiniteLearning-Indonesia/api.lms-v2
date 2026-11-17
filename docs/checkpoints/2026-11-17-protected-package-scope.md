# 17 November — protected local output and authenticated package scope

## Frozen scope before code

User: lanjutkan pekerjaan yang perlu dirampungkan sambil staging disiapkan. Supports M01-CP02 / DLV-03–04; does not invent a new CP or claim full M01. Starting executable fingerprint `469e7d3b57b55a418994173ef73c3a992863bf6f8f72b6b2d8c12176c58ee6ee`. Source/context backup must be reopened/hashed before code edits.

1. Native read-only Windows owner/DACL verification (current user + LocalSystem only; unknown/null/broad ACL fails closed), equivalent owner/mode checks on Linux, unsupported hosts blocked. No modification of any existing ACL, inheritance or owner.
2. Anchored exclusive encrypted output, actual disk capacity, protected identity outside artifact directory, sync + authenticated readback and no receipt on interrupted/corrupt/wrong-key output. Partial files retained, never overwrite/delete/rename/promote. QA may create new synthetic private directories with restrictive ACLs **at initial creation**, without relaxing existing paths or enabling privileges.
3. Explicit authenticated package API/CLI: complete encrypted-artifact coverage, strict bounded receipts, binding to source/snapshot/role/name/package, actual age EOF/digest verification of each payload. Existing hash-only verification and restore planner keep their lower proof status. Authentication never means SQL readability, object recovery, actual DB restore, operational approval or live identity verification.
4. Exact default Go/ops regression, actual pinned-tool crypto + new protected/package cases, Windows build/vet and Linux cross-build. No fake crypto success or changing required tests to hide failure. Current Playwright/DB/remote gates remain separately reported.

Output/key custody refs are references, not independently verified approval. Trusted local administrator/operator-controlled directory assumption remains explicit; cannot defend against malicious machine owner. Default identity support remains one unencrypted X25519 age key. No new root dependency, custom cipher or credential inheritance.

No DB connection/cluster/export/restore/migration, no live/staging, no existing data/file deletion, no proxy/ACL relaxation, no legacy/frontend edits, commit or push. Actual new local database recovery is a later frozen deliverable after these controls pass, not an implicit consequence of CLI authentication.

## Frozen QA assertions

- Default: restrictive ACL policy and unsafe output/key paths rejected; strict auth JSON; missing/duplicate/plaintext/mismatched-role/receipt coverage rejected before tool execution.
- Actual pinned age: protected file exclusive creation/readback/wrong-key/failed-source/no-success receipt; complete encrypted package authenticates while flags stay non-authorizing; altered binding, corrupt payload, wrong key and incorrect coverage do not authenticate. Existing four crypto cases remain mandatory.
- Registry preserves every previous case/shared gate. Missing tools/permission/storage stays BLOCKED or NOT_RUN; no SKIP/PASS substitution.

Execution/results will be recorded separately after source-bound evidence review.
