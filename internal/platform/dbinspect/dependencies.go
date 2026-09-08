package dbinspect

// Metadata only: never select function source/config values, trigger arguments,
// job commands, view definitions, role passwords or foreign-server options.
const dependenciesSQL = `SELECT jsonb_build_object(
 'namespaces',COALESCE((SELECT jsonb_agg(jsonb_build_object(
   'schema',n.nspname,'current_role_usage',pg_catalog.has_schema_privilege(n.oid,'USAGE'),
   'current_role_create',pg_catalog.has_schema_privilege(n.oid,'CREATE'),
   'relation_count',(SELECT count(*) FROM pg_catalog.pg_class c WHERE c.relnamespace=n.oid)) ORDER BY n.nspname)
   FROM pg_catalog.pg_namespace n WHERE n.nspname NOT LIKE 'pg_%' AND n.nspname<>'information_schema'),'[]'::jsonb),
 'relations',COALESCE((SELECT jsonb_agg(jsonb_build_object('schema',n.nspname,'name',c.relname,'kind',c.relkind)
   ORDER BY n.nspname,c.relname) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname NOT LIKE 'pg_%' AND n.nspname<>'information_schema' AND c.relkind IN ('r','p','v','m','f','S')),'[]'::jsonb),
 'triggers',COALESCE((SELECT jsonb_agg(jsonb_build_object('schema',n.nspname,'table',c.relname,
   'name',t.tgname,'enabled',t.tgenabled,'internal',t.tgisinternal,'function_oid',t.tgfoid,
   'definition_md5',pg_catalog.md5(pg_catalog.pg_get_triggerdef(t.oid))) ORDER BY n.nspname,c.relname,t.tgname)
   FROM pg_catalog.pg_trigger t JOIN pg_catalog.pg_class c ON c.oid=t.tgrelid
   JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname NOT LIKE 'pg_%' AND n.nspname<>'information_schema'),'[]'::jsonb),
 'routines',COALESCE((SELECT jsonb_agg(jsonb_build_object('schema',n.nspname,'name',p.proname,
   'oid',p.oid,'argument_types',p.proargtypes::text,'kind',p.prokind,'security_definer',p.prosecdef,
   'owner_oid',p.proowner,'config_md5',pg_catalog.md5(COALESCE(p.proconfig::text,'')),
   'body_md5',pg_catalog.md5(COALESCE(p.prosrc,'') || COALESCE(p.prosqlbody::text,'')),
   'anon_execute',(SELECT pg_catalog.has_function_privilege(r.oid,p.oid,'EXECUTE') FROM pg_catalog.pg_roles r WHERE r.rolname='anon'),
   'authenticated_execute',(SELECT pg_catalog.has_function_privilege(r.oid,p.oid,'EXECUTE') FROM pg_catalog.pg_roles r WHERE r.rolname='authenticated'))
   ORDER BY n.nspname,p.proname,p.oid) FROM pg_catalog.pg_proc p
   JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace
   WHERE n.nspname NOT LIKE 'pg_%' AND n.nspname<>'information_schema'),'[]'::jsonb),
 'api_role_table_privileges',COALESCE((SELECT jsonb_agg(jsonb_build_object('schema',n.nspname,'table',c.relname,'role',r.rolname,
   'schema_usage',pg_catalog.has_schema_privilege(r.oid,n.oid,'USAGE'),
   'select',pg_catalog.has_table_privilege(r.oid,c.oid,'SELECT'),
   'insert',pg_catalog.has_table_privilege(r.oid,c.oid,'INSERT'),
   'update',pg_catalog.has_table_privilege(r.oid,c.oid,'UPDATE'),
   'delete',pg_catalog.has_table_privilege(r.oid,c.oid,'DELETE'),
   'truncate',pg_catalog.has_table_privilege(r.oid,c.oid,'TRUNCATE')) ORDER BY n.nspname,c.relname,r.rolname)
   FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
   CROSS JOIN pg_catalog.pg_roles r WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m')
   AND r.rolname IN ('anon','authenticated')),'[]'::jsonb),
 'scheduler_scope','CATALOG_PRESENCE_ONLY_NO_JOB_ROWS',
 'scheduler_relations',COALESCE((SELECT jsonb_agg(jsonb_build_object('schema',n.nspname,'relation',c.relname) ORDER BY n.nspname,c.relname)
   FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname IN ('cron','pgmq') AND c.relkind IN ('r','p','v')),'[]'::jsonb))`
