# Database access model — proposed, not provisioned

Checkpoint CP02; refreshed in [CP07 handoff](CP07_ENVIRONMENT_SECURITY_HANDOFF.md). Current staging credential is an owner/write-capable role; inspector's READ ONLY transaction does not remove those rights. No shared-database GRANT/REVOKE/RLS/role changes performed. CP06 creates a restricted role only in its fresh synthetic local cluster.

| Principal | Intended scope | Explicit exclusions / review |
|---|---|---|
| Metadata inspector | Catalog SELECT, approved namespace metadata | No business rows, owner, BYPASSRLS, DDL, role management or write |
| Backup operator | Complete approved source snapshot and object scope | Do not silently accept RLS-filtered/partial dumps; prove completeness and approve sensitive access separately |
| Go runtime | Approved SELECT/INSERT/UPDATE in v3 namespace; contextual application capabilities | No ownership, BYPASSRLS, DELETE/TRUNCATE/DDL, legacy writes or broad function EXECUTE |
| Migration operator | Reviewed additive commands on exact v3 target | Backup/restore receipt, checksum, lock, operator approval; no destructive commands |

Do not replace legacy credentials suddenly. Review default ACLs, memberships, schema USAGE, routines/SECURITY DEFINER, and Data API exposed schemas together. Public RLS=false plus API-role grants is a configuration risk, not proof that a public endpoint was tested/exploited. Raw function bodies/trigger args/job commands can contain secrets and are not exported by the inspector.

CP02 inventory adds non-system namespace/relation metadata, triggers, routine flags/hashes and effective public-table privileges for existing anon/authenticated roles. Scheduler catalog presence is NOT a job inventory. External scheduler config, function semantics, storage references/object bytes, provider API exposure settings, production catalog and deployed SHA remain pending. No private function is invoked.

Production input needed: actual deployment secret reference, project/database identity and deployed artifact SHA. The four local env fingerprints do not establish two distinct databases. Do not infer production identity from folder names.

References: [Supabase API security](https://supabase.com/docs/guides/api/securing-your-api), [PostgreSQL pg_proc](https://www.postgresql.org/docs/17/catalog-pg-proc.html).
