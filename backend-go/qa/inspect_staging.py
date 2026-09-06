"""Explicit operator probe, NOT part of automatic synthetic QA/CI.

Reads only ROOT/.env.staging, compiles Go CLI, captures sanitized metadata.
No import, dump, migration, backup claim, grant/revoke, or row reads.
"""
import argparse
from datetime import datetime, timezone
from hashlib import sha256
import json
import os
from pathlib import Path
import sys
import uuid

from qa.evidence import source_fingerprint, write_new_json
from qa.observations import observation_digest
from qa.run import ROOT, clean_environment, run_command, run_lock


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--staging-readonly", action="store_true", required=True)
    parser.parse_args()
    parent = ROOT / ".qa-artifacts"
    parent.mkdir(exist_ok=True)
    try:
        with run_lock(parent / "staging-readonly.lock"):
            run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-staging-inspect-" + uuid.uuid4().hex[:12]
            directory = parent / run_id
            directory.mkdir()
            source = source_fingerprint(ROOT)
            executable = directory / ("db-inspect.exe" if os.name == "nt" else "db-inspect")
            env = clean_environment(ROOT, os.environ)
            build = run_command(["go", "build", "-trimpath", "-o", str(executable), "./cmd/db-inspect"],
                                env, directory, "build-db-inspect")
            if build["return_code"] != 0:
                print("Probe build failed; no database connection attempted. Evidence: " + str(directory))
                return 1
            config_path = ROOT / ".env.staging"
            probe = run_command([str(executable), "--env-file", str(config_path), "--staging-readonly"],
                                env, directory, "db-inspect", 35)
            observation = json.loads(probe["stdout"]) if probe["stdout"].strip() else {
                "status": "BLOCKED", "blockers": ["CONFIG_OR_PROBE_UNAVAILABLE"], "database_mutations": "NONE"}
            result = {"version": 1, "run_id": run_id, "source_hash": source,
                      "current_source_hash": source_fingerprint(ROOT),
                      "binary_sha256": sha256(executable.read_bytes()).hexdigest(),
                      "config_reference": ".env.staging (user-identified staging)",
                      "probe_exit_code": probe["return_code"], "observation": observation,
                      "observation_sha256": observation_digest(observation),
                      "backup": "NOT_EXECUTED", "migration": "NOT_EXECUTED", "release": "NOT_VERIFIED"}
            write_new_json(directory / "summary.json", result)
            print(json.dumps({"run_id": run_id, "status": observation["status"],
                              "tls_verified": observation.get("tls_verified", False),
                              "read_only_transaction": observation.get("read_only_transaction", False),
                              "public_table_count": len(observation["public_tables"]) if "public_tables" in observation else None,
                              "blockers": observation.get("blockers", [])}, indent=2))
            print("Private evidence: " + str(directory / "summary.json"))
            if source != result["current_source_hash"]:
                print("Source changed during observation; evidence is stale.")
                return 1
            return 0 if probe["return_code"] == 0 and observation["status"] == "READ_ONLY_OBSERVATION_COMPLETE" else 2
    except (OSError, ValueError, KeyError):
        print("Probe refused: unavailable scope/lock/evidence; no operational approval inferred.", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
