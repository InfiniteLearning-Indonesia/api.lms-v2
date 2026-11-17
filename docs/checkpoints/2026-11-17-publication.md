# 17 November — public source publication scope

User explicitly requested publishing latest updates to GitHub `be-v3` **before any shared database access**. This authorization supersedes earlier no-commit/no-push constraints for this publication only; database work remains outside scope.

Repository/worktree root is `backend-v3/backend-go`, remote `InfiniteLearning-Indonesia/api.lms-v2`, branch `be-v3`. Baseline local and remote ref observed at `7e7e86b2fa6408d8945f9d2e983cdcb76a27a2d2`. Publish Go source-root paths directly, not the surrounding legacy workspace or parent folder layout. No force push/history rewrite intended. Final commit/ref verification is recorded in private publication receipts and Git history, not a self-referential SHA in this document.

Package: checksum-bound preflight/package validation, pinned age tooling, protected output/authenticated package/local transfer, small local recovery QA runner, registry/public helper/CI repairs and reconciled module/baseline/staging handoff docs. [Current recovery evidence](2026-11-17-local-recovery.md) stays **IMPLEMENTED_PENDING_OWNER_NATIVE_QA**: source `454187dda3c48e32681a73c9810d47d92fee196e17f00958a2898df608bdfa33`, 81/81 Go and 95/95 ops/vet/build PASS, current native 4 PASS + 5 fixture AccessDenied FAIL, ten DB recovery stages NOT_RUN. [Previous seven-case owner acceptance](2026-11-17-protected-package-accepted.md) remains historical on its tested source.

Publish no `.env*`, credentials, key/CA material, raw logs/receipts, SQL/custom dumps, physical databases, local retention policy/pins, caches, executables or private backups. Public JSON files are contracts/manifests/examples, not database contents or secret values. Parent portability notes are local companion material; canonical public context is in this repo's docs.

User clarification: `.env.staging` now identifies the intended real staging database with data synchronized from main live. This is **user-declared environment context**, not independent TLS/identity/role/schema/consistency evidence. Credentials are neither copied here nor loaded for publication. No staging/live connection, metadata query, migration, restore, deployment or traffic switch is authorized by this commit.

The existing push workflow runs synthetic QA only, with no shared database secrets/deploy step. Publishing source is not passing CI, native recovery, full M01/DLV-04/DLV-06 readiness or product rollout. Preserve failed/history evidence; missing native/remote gates remain open.
