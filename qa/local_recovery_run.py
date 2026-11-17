"""Small fresh-local PostgreSQL recovery QA. No DSN/env/existing cluster input.

Normal owner terminal only. Keeps backups, evidence and stopped cluster;
never deletes, relaxes ACLs, runs retention disposal or uses real providers.
"""
import argparse
import ctypes
from datetime import datetime, timezone
from hashlib import sha256
import json
import os
from pathlib import Path
import re
import secrets
import socket
import subprocess
import sys
import time
import uuid

from qa import age_stream_run, storage
from qa.evidence import source_fingerprint
from qa.run import ROOT, clean_environment, run_lock
from qa.preparation import verify_ops_cases

DB_CASES = ['source-prestate-backup-restore', 'target-prestate-backup-restores',
            'consistent-custom-and-sql', 'actual-package-authentication', 'protected-payload-recovery',
            'custom-restore', 'sql-restore', 'schema-rows-sequences-files-parity',
            'foreign-key-denial-retains-state', 'normal-stop']
TOOLS = ('initdb','pg_ctl','pg_dump','pg_restore','psql','createdb','pg_controldata')
DATABASES = {'qa_recovery_source','v3_restore_custom','v3_restore_sql',
             'source_empty_proof','custom_empty_proof','sql_empty_proof'}
TABLES = ('marker','classes','learners','enrollments')
CATALOG = [
    "SELECT c.relname,c.relkind FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='qa_recovery' ORDER BY 1,2",
    "SELECT table_name,column_name,ordinal_position,data_type,is_nullable,column_default FROM information_schema.columns WHERE table_schema='qa_recovery' ORDER BY 1,3",
    "SELECT c.relname,k.conname,pg_get_constraintdef(k.oid) FROM pg_constraint k JOIN pg_class c ON c.oid=k.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='qa_recovery' ORDER BY 1,2",
    "SELECT indexname,indexdef FROM pg_indexes WHERE schemaname='qa_recovery' ORDER BY 1",
]
FIXTURE = """CREATE SCHEMA qa_recovery;
CREATE TABLE qa_recovery.marker(id text PRIMARY KEY, value text NOT NULL);
CREATE TABLE qa_recovery.classes(id bigserial PRIMARY KEY, name text NOT NULL);
CREATE TABLE qa_recovery.learners(id bigserial PRIMARY KEY, name text NOT NULL);
CREATE TABLE qa_recovery.enrollments(id bigserial PRIMARY KEY,
 learner_id bigint NOT NULL REFERENCES qa_recovery.learners(id),
 class_id bigint NOT NULL REFERENCES qa_recovery.classes(id), UNIQUE(learner_id,class_id));
CREATE INDEX enrollments_class_idx ON qa_recovery.enrollments(class_id);
INSERT INTO qa_recovery.classes(name) VALUES ('Class recovery fixture');
INSERT INTO qa_recovery.learners(name) VALUES ('Laras'),('Rafi');
INSERT INTO qa_recovery.enrollments(learner_id,class_id) VALUES (1,1),(2,1);
"""


def manifest(service=ROOT):
    v=json.loads((service/'qa/manifests/M01-LOCAL-RECOVERY.json').read_text(encoding='utf8'))
    if v.get('database_cases')!=DB_CASES or v.get('scope')!='NEW_LOCAL_SYNTHETIC_RECOVERY_ONLY' or v.get('full_module_verified') is not False:
        raise ValueError('RECOVERY_MANIFEST_INVALID')
    return v


def database_name(value):
    if value not in DATABASES: raise ValueError('RECOVERY_DATABASE_OUTSIDE_FIXED_SET')
    return value


def private_mkdir(path):
    """Owner/SYSTEM DACL at creation only. No SetSecurityInfo/ACL repair."""
    if path.exists() or any(storage.reparse(p) for p in path.parents if p.exists()):
        raise ValueError('RECOVERY_PRIVATE_PATH_EXISTS_OR_LINKED')
    if os.name!='nt':
        path.mkdir(mode=0o700);return
    from ctypes import wintypes as w
    k=ctypes.WinDLL('kernel32',use_last_error=True);a=ctypes.WinDLL('advapi32',use_last_error=True)
    k.GetCurrentProcess.restype=w.HANDLE;k.CloseHandle.argtypes=[w.HANDLE]
    a.OpenProcessToken.argtypes=[w.HANDLE,w.DWORD,ctypes.POINTER(w.HANDLE)]
    a.GetTokenInformation.argtypes=[w.HANDLE,w.DWORD,ctypes.c_void_p,w.DWORD,ctypes.POINTER(w.DWORD)]
    a.ConvertSidToStringSidW.argtypes=[ctypes.c_void_p,ctypes.POINTER(ctypes.c_void_p)]
    a.ConvertStringSecurityDescriptorToSecurityDescriptorW.argtypes=[w.LPCWSTR,w.DWORD,ctypes.POINTER(ctypes.c_void_p),ctypes.c_void_p]
    k.LocalFree.argtypes=[ctypes.c_void_p]
    token=w.HANDLE();needed=w.DWORD();sid_text=ctypes.c_void_p();descriptor=ctypes.c_void_p()
    class Attributes(ctypes.Structure):
        _fields_=[('length',w.DWORD),('descriptor',ctypes.c_void_p),('inherit',w.BOOL)]
    k.CreateDirectoryW.argtypes=[w.LPCWSTR,ctypes.POINTER(Attributes)]
    if not a.OpenProcessToken(k.GetCurrentProcess(),8,ctypes.byref(token)):raise ValueError('RECOVERY_TOKEN_QUERY_FAILED')
    try:
        a.GetTokenInformation(token,1,None,0,ctypes.byref(needed))
        if not 0<needed.value<65536:raise ValueError('RECOVERY_TOKEN_SIZE_INVALID')
        buf=ctypes.create_string_buffer(needed.value)
        if not a.GetTokenInformation(token,1,buf,needed,ctypes.byref(needed)):raise ValueError('RECOVERY_TOKEN_QUERY_FAILED')
        sid=ctypes.cast(buf,ctypes.POINTER(ctypes.c_void_p))[0]
        if not a.ConvertSidToStringSidW(sid,ctypes.byref(sid_text)):raise ValueError('RECOVERY_SID_FAILED')
        sddl='O:'+ctypes.wstring_at(sid_text)+'D:P(A;OICI;FA;;;'+ctypes.wstring_at(sid_text)+')(A;OICI;FA;;;SY)'
        if not a.ConvertStringSecurityDescriptorToSecurityDescriptorW(sddl,1,ctypes.byref(descriptor),None):raise ValueError('RECOVERY_DESCRIPTOR_FAILED')
        attr=Attributes(ctypes.sizeof(Attributes),descriptor,False)
        if not k.CreateDirectoryW(str(path),ctypes.byref(attr)):raise ValueError('RECOVERY_PRIVATE_DIRECTORY_FAILED')
    finally:
        if descriptor.value:k.LocalFree(descriptor)
        if sid_text.value:k.LocalFree(sid_text)
        k.CloseHandle(token)


def write_new(path,data):
    fd=os.open(path,os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600)
    with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())


def operator_regression(service,root,env):
    binaries=root/'bin';binaries.mkdir();extension='.exe' if os.name=='nt' else ''
    child=dict(env,QA_ARTIFACT_ROOT=str(root),QA_CASE_RESULTS=str(root/'ops-cases.json'))
    for name,key in [('db-inspect','QA_DB_INSPECT_BINARY'),('baseline-preflight','QA_BASELINE_PREFLIGHT_BINARY'),('baseline-package','QA_BASELINE_PACKAGE_BINARY')]:
        binary=binaries/(name+extension)
        built=subprocess.run(['go','build','-trimpath','-o',str(binary),'./cmd/'+name],cwd=service,env=env,capture_output=True,timeout=120)
        if built.returncode:raise ValueError('RECOVERY_OPS_BINARY_BUILD_FAILED')
        child[key]=str(binary)
    with (root/'ops.stdout').open('xb') as out,(root/'ops.stderr').open('xb') as error:
        checked=subprocess.run([sys.executable,'-m','pytest','-p','qa.plugin','-p','no:cacheprovider','-q','qa/tests/ops'],cwd=service,env=child,stdout=out,stderr=error,timeout=300)
    frozen=json.loads((service/'qa/manifests/M03.json').read_text())
    excluded={'Q00-go-format','Q00-go-build','Q00-go-unit','Q00-go-vet'}
    expected=[v for v in frozen['expected_checks'] if v not in excluded and not v.startswith('M03-T01-')]
    if checked.returncode or verify_ops_cases(json.loads((root/'ops-cases.json').read_text()),expected)!=len(expected):raise ValueError('RECOVERY_OPS_NOT_PASS')
    storage.dump_new(root/'preparation.json',{'ops_exact_count':len(expected),'status':'PASS_OPS_ONLY','database_access':'NONE'})
    storage.guard_growth(root)


def run(pg_bin, acknowledged=False, service=ROOT):
    if not acknowledged:raise ValueError('RECOVERY_EXPLICIT_NEW_CLUSTER_ACK_REQUIRED')
    manifest(service)
    source=source_fingerprint(service)
    print('[recovery] mandatory current crypto/Go gate first; no database yet',flush=True)
    if age_stream_run.execute(service=service,allow_install=False,max_minutes=30)!=0:
        raise ValueError('RECOVERY_CRYPTO_GATE_NOT_PASS')
    if source!=source_fingerprint(service):raise ValueError('RECOVERY_SOURCE_CHANGED')
    pg_bin=Path(pg_bin).resolve(strict=True)
    if any(storage.reparse(p) for p in (pg_bin,*pg_bin.parents)):raise ValueError('RECOVERY_LINKED_TOOL_PATH')
    ext='.exe' if os.name=='nt' else ''
    tools={n:pg_bin/(n+ext) for n in TOOLS}
    if any(not p.is_file() or storage.reparse(p) for p in tools.values()):raise ValueError('RECOVERY_PG18_TOOLS_REQUIRED')
    base_env=clean_environment(service,os.environ);versions={}
    for name,path in tools.items():
        v=subprocess.run([str(path),'--version'],env=base_env,capture_output=True,text=True,timeout=15)
        if v.returncode or not re.search(r'\b18(?:\.|\b)',v.stdout):raise ValueError('RECOVERY_PG18_VERSION_REQUIRED')
        versions[name]=v.stdout.strip()
    parent=storage.project_locations(service)[0];parent.mkdir(exist_ok=True)
    with run_lock(parent/'storage.lock'),run_lock(parent/'postgres-lab.lock'):
        print('[recovery] read-only managed storage admission; no cleanup',flush=True)
        admission=storage.preflight(service,parent)
        run_id=datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'-postgres-lab-'+uuid.uuid4().hex[:12]
        root=parent/run_id;root.mkdir()
        storage.dump_new(root/'storage-preflight.json',admission)
        storage.dump_new(root/'retention-pin.json',{'version':1,'reason':'Small protected-recovery profile; retain pending explicit policy/acceptance review. No disposal authorized by this runner.'})
        storage.dump_new(root/'tools.json',versions)
        storage.dump_new(root/'creation.json',{'run_id':run_id,'source_hash':source,'scope':'NEW_LOCAL_SYNTHETIC_RECOVERY_ONLY','database_access':'NONE_YET'})
        print('[recovery] new run '+run_id+'; evidence '+str(root),flush=True)
        operator_regression(service,root,base_env)
        if source!=source_fingerprint(service):raise ValueError('RECOVERY_SOURCE_CHANGED')
        deadline=time.monotonic()+20*60
        data=root/'postgres';private=root/'private';private_mkdir(private)
        for n in ('keys','source','package','recovered'):private_mkdir(private/n)
        admin_password=secrets.token_hex(32);app_password=secrets.token_hex(32);marker=secrets.token_hex(32)
        password_file=private/'keys/bootstrap.private';write_new(password_file,admin_password.encode())
        with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
        common=['--host=127.0.0.1','--port='+str(port),'--no-password']
        env=dict(base_env,PGPASSWORD=admin_password,PGCONNECT_TIMEOUT='10',PGSSLMODE='disable',PGOPTIONS='-c statement_timeout=30000')
        app_env=dict(env,PGPASSWORD=app_password)
        result={'run_id':run_id,'source_hash':source,'status':'FAIL','profile':'new-local-synthetic',
                'scope':'NEW_LOCAL_SYNTHETIC_RECOVERY_ONLY','shared_database_access':'NONE','database_deletion':'NONE',
                'production_backup':'NOT_EXECUTED','actual_restore_verified':False,'real_baseline_verified':False,
                'full_module_verified':False,'migration_authority':False,'cluster_retained':True,'cases':[]}
        calls=[];counter=0;started=False

        def case(name):
            if name not in DB_CASES or any(c['id']==name for c in result['cases']):raise ValueError('RECOVERY_CASE_SET_INVALID')
            result['cases'].append({'id':name,'status':'pass'})

        def command(tool,args,child_env=None,input_bytes=None,expect_failure=False,timeout=60,cleanup=False):
            nonlocal counter
            counter+=1
            remaining=int(deadline-time.monotonic())
            if not cleanup and remaining<1:raise ValueError('RECOVERY_TIME_BUDGET_EXHAUSTED')
            out=root/f'command-{counter:03}.stdout';err=root/f'command-{counter:03}.stderr'
            with out.open('xb') as stdout,err.open('xb') as stderr:
                p=subprocess.run([str(tools[tool]),*[str(x) for x in args]],env=child_env or env,cwd=service,
                                 input=input_bytes,stdout=stdout,stderr=stderr,timeout=timeout if cleanup else min(timeout,remaining))
            stdout=out.read_bytes();stderr=err.read_bytes()
            calls.append({'tool':tool,'exit_code':p.returncode,'expected_failure':expect_failure})
            if (p.returncode!=0)!=expect_failure:raise ValueError('RECOVERY_COMMAND_FAILED_'+tool.upper())
            if tool=='initdb' and b'initdb: error:' in stderr.lower():raise ValueError('RECOVERY_INITDB_FATAL_DESPITE_EXIT_ZERO')
            storage.guard_growth(root)
            if source!=source_fingerprint(service):raise ValueError('RECOVERY_SOURCE_CHANGED')
            return stdout,stderr

        def sql(db,text,admin=False,expect_failure=False):
            database_name(db) if db!='postgres' else None
            return command('psql',[*common,'--username='+('qa_recovery_admin' if admin else 'qa_recovery_app'),'--dbname='+db,
                     '-X','-q','-A','-t','--file=-','--single-transaction','-v','ON_ERROR_STOP=1','-v','VERBOSITY=sqlstate'],
                     env if admin else app_env,text.encode(),expect_failure)[0].strip()

        def create_db(db):
            command('createdb',[*common,'--username=qa_recovery_admin','--maintenance-db=postgres','--owner=qa_recovery_app',database_name(db)])

        def fingerprint(db,empty=False):
            queries=list(CATALOG)
            if not empty:
                queries += [f'SELECT to_jsonb(r)::text FROM qa_recovery.{t} r ORDER BY 1' for t in TABLES]
                queries += [f'SELECT last_value,is_called FROM qa_recovery.{t}_id_seq' for t in ('classes','learners','enrollments')]
            return [sha256(sql(db,q)).hexdigest() for q in queries]

        def empty_backup(db,label):
            if sql(db,"SELECT count(*) FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema')")!=b'0':raise ValueError('RECOVERY_TARGET_NOT_EMPTY')
            dump=root/(label+'.dump');rendered=root/(label+'.sql')
            command('pg_dump',[*common,'--username=qa_recovery_app','--dbname='+db,'--format=custom','--no-owner','--no-acl','--file='+str(dump)],app_env)
            script,_=command('pg_restore',['--no-owner','--no-acl','--file=-',dump]);write_new(rendered,script)
            proof=label+'_proof';create_db(proof)
            command('pg_restore',[*common,'--username=qa_recovery_app','--dbname='+proof,'--single-transaction','--exit-on-error','--no-owner','--no-acl',dump],app_env)
            if fingerprint(db,True)!=fingerprint(proof,True):raise ValueError('RECOVERY_EMPTY_RESTORE_MISMATCH')
            receipt={'scope':'SYNTHETIC_ONLY','restore':'VERIFIED_FIXTURE_SCOPE','source_database':db,'proof_database':proof,
                     'artifacts':{p.name:storage.file_hash(p) for p in (dump,rendered)}}
            storage.dump_new(root/(label+'-receipt.json'),receipt)

        try:
            print('[recovery] creating NEW loopback synthetic PostgreSQL cluster',flush=True)
            command('initdb',['-D',data,'--username=qa_recovery_admin','--pwfile='+str(password_file),
                             '--auth-host=scram-sha-256','--auth-local=scram-sha-256','--encoding=UTF8','--no-locale','--no-clean'])
            if any(not (data/name).is_file() for name in ('PG_VERSION','pg_hba.conf','postgresql.conf','global/pg_control')) or (data/'PG_VERSION').read_text().strip()!='18':raise ValueError('RECOVERY_CLUSTER_INCOMPLETE')
            started=True
            command('pg_ctl',['-D',data,'-l',root/'postgres.log','-o',f'-h 127.0.0.1 -p {port} -c ssl=off -c max_connections=8 -c shared_buffers=16MB -c max_wal_size=64MB -c min_wal_size=32MB','-w','-t','20','start'])
            directory=sql('postgres',"SELECT current_setting('data_directory')",admin=True).decode()
            if Path(directory).resolve()!=data.resolve():raise ValueError('RECOVERY_SERVER_IDENTITY_MISMATCH')
            sql('postgres',"CREATE ROLE qa_recovery_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS PASSWORD '"+app_password+"'",admin=True)
            for db in ('qa_recovery_source','v3_restore_custom','v3_restore_sql'):create_db(db)
            if sql('qa_recovery_source',"SELECT rolsuper,rolcreatedb,rolcreaterole,rolbypassrls FROM pg_roles WHERE rolname=current_user")!=b'f|f|f|f':raise ValueError('RECOVERY_ROLE_NOT_RESTRICTED')
            empty_backup('qa_recovery_source','source_empty');case(DB_CASES[0])
            empty_backup('v3_restore_custom','custom_empty');empty_backup('v3_restore_sql','sql_empty');case(DB_CASES[1])
            sql('qa_recovery_source',FIXTURE+f"INSERT INTO qa_recovery.marker VALUES ('run','{marker}');")
            for table,count in zip(TABLES,(1,1,2,2)):
                if sql('qa_recovery_source','SELECT count(*) FROM qa_recovery.'+table)!=str(count).encode():raise ValueError('RECOVERY_FIXTURE_INVENTORY_MISMATCH')
            original=fingerprint('qa_recovery_source')
            dump=private/'source/database.dump';script=private/'source/database.sql'
            command('pg_dump',[*common,'--username=qa_recovery_app','--dbname=qa_recovery_source','--format=custom','--no-owner','--no-acl','--file='+str(dump)],app_env)
            command('pg_restore',['--list',dump]);command('pg_restore',['--no-owner','--no-acl','--file='+str(script),dump])
            if original!=fingerprint('qa_recovery_source'):raise ValueError('RECOVERY_SOURCE_CHANGED_DURING_SNAPSHOT')
            case(DB_CASES[2])
            proof=json.loads((service/'.cache/qa-tools/age-v1.3.2/verified-install.json').read_text())
            toolroot,age,keygen=age_stream_run.tool_paths(service)
            if any(storage.file_hash(p)!=proof[p.name] for p in (age,keygen)):raise ValueError('RECOVERY_AGE_TOOL_CHANGED')
            key_env={k:v for k,v in base_env.items() if k.upper() in {'SYSTEMROOT','WINDIR','TEMP','TMP','TMPDIR'}}
            keys=subprocess.run([str(keygen)],env=key_env,capture_output=True,timeout=30)
            if keys.returncode or not 0<len(keys.stdout)<4096:raise ValueError('RECOVERY_KEYGEN_FAILED')
            identity=private/'keys/identity.txt';write_new(identity,keys.stdout)
            recipients=[l.removeprefix('# public key: ') for l in keys.stdout.decode().splitlines() if l.startswith('# public key: ')]
            if len(recipients)!=1:raise ValueError('RECOVERY_RECIPIENT_MISSING')
            object_bytes=b'Synthetic recovered object fixture; never a live bucket object.'
            write_new(private/'source/objects.bundle',object_bytes)
            package,auth=build_package(root,private,marker,port,proof,age,identity,original)
            binary=root/('baseline-package'+ext)
            build=subprocess.run(['go','build','-trimpath','-o',str(binary),'./cmd/baseline-package'],cwd=service,env=base_env,capture_output=True,timeout=120)
            if build.returncode:raise ValueError('RECOVERY_CLI_BUILD_FAILED')
            def cli(flag,value):
                remaining=int(deadline-time.monotonic())
                if remaining<1:raise ValueError('RECOVERY_TIME_BUDGET_EXHAUSTED')
                p=subprocess.run([str(binary),flag],input=json.dumps(value).encode(),env=base_env,capture_output=True,timeout=min(120,remaining))
                if p.returncode:raise ValueError('RECOVERY_CLI_FAILED_'+flag.strip('-').upper().replace('-','_'))
                v=json.loads(p.stdout)
                if v.get('authorizes_execution') or v.get('actual_restore_verified'):raise ValueError('RECOVERY_CLI_OVERCLAIM')
                return v
            for a in package['artifacts'][:3]:
                source_name={'db_custom':'database.dump','db_sql':'database.sql','object_bundle':'objects.bundle'}[a['role']]
                path=private/'source'/source_name;plain={'bytes':path.stat().st_size,'sha256':storage.file_hash(path)}
                v=cli('--seal-only',{'version':1,'package':package,'tool':auth['tool'],'identity_path':str(identity),'role':a['role'],
                       'source_directory':str(private/'source'),'source_name':source_name,'plain':plain,'recipient':recipients[0],'custody_ref':'synthetic-local-custody'})
                receipt=v['receipt'];a['bytes']=receipt['cipher']['bytes'];a['sha256']=receipt['cipher']['sha256']
                auth['artifacts'].append({'role':a['role'],'name':a['name'],'receipt':receipt})
            finalize_metadata(package,private,object_bytes)
            v=cli('--authenticate-only',auth)
            if v.get('authenticated_artifact_count')!=3 or not v.get('archive_authentication_verified'):raise ValueError('RECOVERY_AUTH_NOT_COMPLETE')
            case(DB_CASES[3])
            for a in auth['artifacts']:
                name={'db_custom':'database.dump','db_sql':'database.sql','object_bundle':'objects.bundle'}[a['role']]
                v=cli('--recover-only',{'version':1,'authentication':auth,'role':a['role'],'directory':str(private/'recovered'),'name':name})
                path=private/'recovered'/name
                if v.get('status')!='OFFLINE_ARTIFACT_RECOVERED_NOT_AUTHORIZED' or storage.file_hash(path)!=a['receipt']['plain']['sha256']:raise ValueError('RECOVERY_PLAINTEXT_MISMATCH')
            case(DB_CASES[4])
            storage.dump_new(root/'payload-receipt.json',{'scope':'SYNTHETIC_ONLY','payload_hashes':{p.name:storage.file_hash(p) for p in (private/'recovered').iterdir()},'package_plan_sha256':v['authenticated_package_sha256']})
            command('pg_restore',[*common,'--username=qa_recovery_app','--dbname=v3_restore_custom','--single-transaction','--exit-on-error','--no-owner','--no-acl',private/'recovered/database.dump'],app_env);case(DB_CASES[5])
            command('psql',[*common,'--username=qa_recovery_app','--dbname=v3_restore_sql','-X','--single-transaction','-v','ON_ERROR_STOP=1','--file='+str(private/'recovered/database.sql')],app_env);case(DB_CASES[6])
            for db in ('v3_restore_custom','v3_restore_sql'):
                if fingerprint(db)!=original:raise ValueError('RECOVERY_RESTORED_PARITY_MISMATCH')
            if (private/'recovered/objects.bundle').read_bytes()!=object_bytes:raise ValueError('RECOVERY_OBJECT_MISMATCH')
            case(DB_CASES[7])
            sql('v3_restore_custom','INSERT INTO qa_recovery.enrollments(id,learner_id,class_id) VALUES(999,999,1)',expect_failure=True)
            if b'23503' not in (root/f'command-{counter:03}.stderr').read_bytes() or fingerprint('v3_restore_custom')!=original:raise ValueError('RECOVERY_FK_DENIAL_INVALID')
            case(DB_CASES[8]);result['parity_sha256']=sha256(json.dumps(original).encode()).hexdigest()
            storage.dump_new(root/'recovery-snapshot-receipt.json',{'scope':'SYNTHETIC_ONLY','restore':'VERIFIED_FIXTURE_SCOPE',
                 'source_database':'qa_recovery_source','proof_database':'v3_restore_custom','artifact_sha256':storage.file_hash(dump),
                 'sql_sha256':storage.file_hash(script),'restored_schema_rows_sequence_fingerprints':original})
        except (Exception,KeyboardInterrupt) as error:
            result['failure']=str(error) if isinstance(error,ValueError) and re.fullmatch('[A-Z0-9_]{1,100}',str(error)) else type(error).__name__
        finally:
            if started:
                try:
                    command('pg_ctl',['-D',data,'-m','fast','-w','-t','30','stop'],cleanup=True)
                    control,_=command('pg_controldata',['-D',data],cleanup=True)
                    if (data/'postmaster.pid').exists() or not re.search(rb'Database cluster state:\s+shut down',control):raise ValueError('RECOVERY_STOP_NOT_PROVEN')
                    case(DB_CASES[9])
                except (Exception,KeyboardInterrupt):result['failure']='RECOVERY_STOP_REQUIRES_OPERATOR'
            try:
                result['storage_after_run']=storage.measure(root)
                storage.guard_growth(root)
            except (OSError,ValueError):
                result.setdefault('failure','RECOVERY_STORAGE_REVIEW_REQUIRED')
            if sorted(c['id'] for c in result['cases'])==sorted(DB_CASES) and not result.get('failure') and source==source_fingerprint(service):
                result.update(status='PASS_SYNTHETIC_ONLY',actual_restore_verified=True)
            result['commands']=calls
            storage.dump_new(root/'recovery-cases.json',{'cases':result['cases']})
            storage.dump_new(root/'summary.json',result)
        print(json.dumps({'status':result['status'],'run_id':run_id,'report':str(root),'cases':len(result['cases']),
                          'actual_restore_verified':result['actual_restore_verified'],'scope':result['scope'],'full_module_verified':False}),flush=True)
        return 0 if result['status']=='PASS_SYNTHETIC_ONLY' else 2


def build_package(root,private,marker,port,proof,age,identity,original):
    now=datetime.now(timezone.utc).isoformat();source_id=sha256(('synthetic-source'+marker+str(port)).encode()).hexdigest()
    schema=sha256(json.dumps(original[:4]).encode()).hexdigest();target=sha256(('synthetic-target'+marker).encode()).hexdigest()
    intake={'version':1,'run_id':root.name,'source':{'identity_sha256':source_id,'schema_sha256':schema,'deployed_commit':'a'*40,
            'receipt_ref':'synthetic-not-production','observed_at':now},'target':{'identity_sha256':target,'kind':'new-local-restore','namespace':'recovery-fixture'},
            'schemas':['qa_recovery'],'artifact_root':str(private/'package'),'declarations':{'tls_mode':'verify-full','source_read_only':True,
            'access_reviewed':True,'owner_ref':'synthetic-owner','reviewer_ref':'synthetic-reviewer','approval_ref':'synthetic-not-approval',
            'key_custody_ref':'synthetic-not-independent','artifact_scope_ref':'synthetic-one-object'},
            'capacity':{'dump_bytes':4<<20,'sql_bytes':4<<20,'object_bytes':4<<20,'restore_bytes':16<<20,'reserve_bytes':8<<30}}
    # TLS/read-only/source declarations are fixture predicates, not observations:
    # actual isolated DB uses loopback SCRAM with ssl=off and owns only this run.
    snapshot={'id':'fixture-snapshot','source_identity_sha256':source_id,'schema_sha256':schema,'exported_at':now,
              'consistency_ref':'sql-rendered-from-same-custom','dump_format':'postgres-custom','server_major':18,'dump_client_major':18}
    artifacts=[{'role':role,'name':name,'snapshot_id':snapshot['id'],'bytes':0,'sha256':'','encryption':enc} for role,name,enc in
               [('db_custom','database.dump.age','age-v1'),('db_sql','database.sql.age','age-v1'),('object_bundle','objects.bundle.age','age-v1'),
                ('schema_inventory','schema.json','none'),('file_inventory','files.json','none')]]
    package={'version':1,'intake':intake,'snapshot':snapshot,'artifacts':artifacts}
    return package,{'version':1,'package':package,'tool':{'path':str(age),'sha256':proof[age.name],'module_sum':proof['module_sum']},
                    'identity_path':str(identity),'artifacts':[]}


def finalize_metadata(package,private,object_bytes):
    snapshot=package['snapshot'];source=snapshot['source_identity_sha256']
    catalog={'version':1,'snapshot_id':snapshot['id'],'source_identity_sha256':source,'schema_sha256':snapshot['schema_sha256'],
             'schemas':['qa_recovery'],'tables':[{'schema':'qa_recovery','name':n,'rows':c} for n,c in [('marker',1),('classes',1),('learners',2),('enrollments',2)]],
             'extensions':[{'name':'plpgsql','version':'1.0'}],'complete_ref':'fixture-only-catalog'}
    files={'version':1,'snapshot_id':snapshot['id'],'source_identity_sha256':source,
           'objects':[{'ref':'synthetic-object','bytes':len(object_bytes),'sha256':sha256(object_bytes).hexdigest()}],
           'bundle_sha256':package['artifacts'][2]['sha256'],'no_objects_review_ref':''}
    for a,value in zip(package['artifacts'][3:],[catalog,files]):
        raw=json.dumps(value).encode();write_new(private/'package'/a['name'],raw);a['bytes']=len(raw);a['sha256']=sha256(raw).hexdigest()


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--create-new-local-synthetic-cluster',action='store_true',required=True)
    p.add_argument('--pg-bin',type=Path,required=True)
    a=p.parse_args()
    try:return run(a.pg_bin,a.create_new_local_synthetic_cluster)
    except (OSError,ValueError,subprocess.TimeoutExpired) as error:
        reason=str(error) if isinstance(error,ValueError) and re.fullmatch('[A-Z0-9_]{1,100}',str(error)) else type(error).__name__
        print(json.dumps({'status':'BLOCKED_OR_FAILED_BEFORE_ACCEPTANCE','reason':reason,'shared_database_access':'NONE','actual_restore_verified':False}),flush=True);return 2


if __name__=='__main__':raise SystemExit(main())
