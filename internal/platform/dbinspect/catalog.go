package dbinspect

// Static, catalog-only statements. No caller-supplied SQL, business rows,
// defaults, function bodies, email addresses, tokens, or password hashes.
const identitySQL = `SELECT current_database(), current_user,
 current_setting('server_version_num')::integer,
 current_setting('transaction_read_only'),
 r.rolsuper, r.rolbypassrls, r.rolcreatedb, r.rolcreaterole,
 pg_catalog.has_schema_privilege(current_user, 'public', 'CREATE')
 FROM pg_catalog.pg_roles r WHERE r.rolname=current_user`

const catalogSQL = `SELECT COALESCE(jsonb_agg(t.definition ORDER BY t.name),'[]'::jsonb)
FROM (
 SELECT c.relname AS name, jsonb_build_object(
  'table',c.relname,'kind',c.relkind,'rls',c.relrowsecurity,'force_rls',c.relforcerowsecurity,
  'owned_by_current_role',c.relowner=(SELECT oid FROM pg_catalog.pg_roles WHERE rolname=current_user),
  'can_select',pg_catalog.has_table_privilege(c.oid,'SELECT'),
  'can_insert',pg_catalog.has_table_privilege(c.oid,'INSERT'),
  'can_update',pg_catalog.has_table_privilege(c.oid,'UPDATE'),
  'can_delete',pg_catalog.has_table_privilege(c.oid,'DELETE'),
  'can_truncate',pg_catalog.has_table_privilege(c.oid,'TRUNCATE'),
  'columns',COALESCE((SELECT jsonb_agg(jsonb_build_object(
    'name',a.attname,'type',pg_catalog.format_type(a.atttypid,a.atttypmod),
    'not_null',a.attnotnull,'has_default',a.atthasdef,'position',a.attnum)
    ORDER BY a.attnum) FROM pg_catalog.pg_attribute a
    WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped),'[]'::jsonb),
  'constraints',COALESCE((SELECT jsonb_agg(jsonb_build_object(
    'name',k.conname,'type',k.contype,'columns',k.conkey,'referenced_columns',k.confkey,
    'reference_schema',rn.nspname,'reference_table',rc.relname,
    'on_delete',k.confdeltype,'on_update',k.confupdtype,
    'validated',k.convalidated,'deferrable',k.condeferrable,
    'definition_md5',pg_catalog.md5(pg_catalog.pg_get_constraintdef(k.oid)))
    ORDER BY k.conname) FROM pg_catalog.pg_constraint k
    LEFT JOIN pg_catalog.pg_class rc ON rc.oid=k.confrelid
    LEFT JOIN pg_catalog.pg_namespace rn ON rn.oid=rc.relnamespace
    WHERE k.conrelid=c.oid),'[]'::jsonb),
  'indexes',COALESCE((SELECT jsonb_agg(jsonb_build_object(
    'name',ic.relname,'unique',i.indisunique,'valid',i.indisvalid,
    'definition_md5',pg_catalog.md5(pg_catalog.pg_get_indexdef(i.indexrelid))) ORDER BY ic.relname)
    FROM pg_catalog.pg_index i JOIN pg_catalog.pg_class ic ON ic.oid=i.indexrelid
    WHERE i.indrelid=c.oid),'[]'::jsonb),
  'policies',COALESCE((SELECT jsonb_agg(jsonb_build_object(
    'name',p.polname,'command',p.polcmd,'roles',p.polroles,
    'using_md5',pg_catalog.md5(COALESCE(pg_catalog.pg_get_expr(p.polqual,p.polrelid),'')),
    'check_md5',pg_catalog.md5(COALESCE(pg_catalog.pg_get_expr(p.polwithcheck,p.polrelid),''))) ORDER BY p.polname)
    FROM pg_catalog.pg_policy p WHERE p.polrelid=c.oid),'[]'::jsonb)
 ) AS definition
 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='public' AND c.relkind IN ('r','p')
) t`

const statisticsSQL = `SELECT pg_catalog.pg_database_size(current_database()),
 COALESCE(jsonb_agg(jsonb_build_object('table',c.relname,'estimated_rows',c.reltuples,
 'total_bytes',pg_catalog.pg_total_relation_size(c.oid)) ORDER BY c.relname),'[]'::jsonb)
 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='public' AND c.relkind IN ('r','p')`

const extensionsSQL = `SELECT COALESCE(jsonb_agg(jsonb_build_object('name',extname,'version',extversion) ORDER BY extname),'[]'::jsonb) FROM pg_catalog.pg_extension`
