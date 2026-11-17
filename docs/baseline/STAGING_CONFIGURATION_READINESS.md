# Before editing or activating staging configuration

17 November 2026. No `.env` file was read or edited for this assessment. [Current project handoff](../PROJECT_STATE_BEFORE_LIVE_STAGING.md), [local recovery preparation](LOCAL_RECOVERY_RUNBOOK.md).

**Editing `.env` does not enable a supported Go staging runtime.** Actual [config.Load](../../internal/platform/config/config.go) does not load dotenv files and only permits `local-synthetic` / `v3-local-synthetic`, loopback listeners, explicit direct TLS origin/key in business mode. [cmd/api](../../cmd/api/main.go) uses synthetic issuer/lab databases/Class wiring; many other domains remain opt-in in identity-lab. [labdb](../../internal/labdb/schema.go) requires explicit synthetic acknowledgement/loopback marker. Do not remove these fences or add a staging mode name merely to make a connection work.

| Can be completed without real staging credentials | Status / next gate |
|---|---|
| Verify protected backup/output/package crypto | Seven-case owner run accepted on its old source; changed recovery tree now requires nine cases |
| Local seal/recover APIs and strict CLI | Implemented; current local native QA pending |
| Small fresh local DB/custom/SQL/object recovery runner | Implemented with no DSN/env input; actual native DB result still needed |
| Source/target metadata/intake contract, no-connect planning | Available; declarations are not actual identity/approval proof |
| Review environment/runtime boundaries | This assessment; application config/legacy/runtime files unchanged |
| Full current Playwright/remote CI | Still open; source publication only when requested, no bypass of named-pipe/permission checks |

User may prepare a **separate private staging configuration and metadata**, while leaving it inactive. Before operational use, need actual source/target/deployed identity, dedicated roles/CA/hostname, staging pre-state backup/recovery, schema/extension compatibility and capacity, controlled namespace/single-writer policy, real identity authority and reviewed mapping. Public source/docs get opaque references only, not passwords, DSNs or private keys.

Shared Go runtime composition/configuration must be implemented and tested explicitly: wire domain handlers, correct session/capability authority, private file/provider adapters, readiness/schema gates, secrets/TLS/trusted proxy/browser contract, startup migration/job ownership and outbound isolation. Those changes are separate from this local operator-tooling slice; neither dotenv editing nor a passing fixture activates them.

For baseline analysis, prefer metadata/snapshot supplied by an authorized operator and a **new isolated local restore**. The agent need not receive live credentials or connect Go to live. Staging target preparation and actual pilot/cutover remain later gates; no existing staging overwrite, automatic role provisioning, legacy startup, schema mutation or deployment was performed here.
