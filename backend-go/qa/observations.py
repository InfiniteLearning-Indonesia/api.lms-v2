"""Read-only operational evidence projection; never grants migration authority."""
from datetime import datetime, timezone, timedelta
from hashlib import sha256
import json
from pathlib import Path
import re


def observation_digest(value: dict) -> str:
    return sha256(json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode()).hexdigest()


def reconcile(path: Path, source: str, target: str, now=None) -> dict:
    result = {"status": "REJECTED", "tls": "NOT_VERIFIED", "public_catalog": "NOT_VERIFIED",
              "production_identity": "PENDING", "least_privilege": "PENDING",
              "backup_restore": "PENDING", "full_inventory": "PENDING",
              "authorizes_mutation": False}
    try:
        if not path.is_absolute() or path.is_symlink() or not path.is_file() or path.stat().st_size > 8 * 1024 * 1024:
            raise ValueError()
        raw = path.read_bytes()
        report = json.loads(raw)
        obs = report["observation"]
        run_id = report["run_id"]
        if not re.fullmatch(r"\d{8}T\d{6}Z-staging-inspect-[a-f0-9]{12}", run_id):
            raise ValueError()
        if not re.fullmatch(r"[a-f0-9]{64}", target) or obs["connection_fingerprint"] != target:
            raise ValueError()
        if report["source_hash"] != source or report["current_source_hash"] != source:
            result["status"] = "STALE_SOURCE"
            return result
        observed = datetime.fromisoformat(obs["observed_at"].replace("Z", "+00:00"))
        age = (now or datetime.now(timezone.utc)) - observed
        if not timedelta(0) <= age <= timedelta(hours=24):
            result["status"] = "STALE_TIME"
            return result
        if (report.get("observation_sha256") != observation_digest(obs)
                or report["probe_exit_code"] != 0
                or report["config_reference"] != ".env.staging (user-identified staging)"
                or obs["environment"] != "USER_IDENTIFIED_STAGING"
                or obs["status"] != "READ_ONLY_OBSERVATION_COMPLETE"
                or obs["tls_verified"] is not True or obs["read_only_transaction"] is not True
                or obs["database_mutations"] != "NONE"
                or report["backup"] != "NOT_EXECUTED" or report["migration"] != "NOT_EXECUTED"
                or not isinstance(obs["public_tables"], list)
                or not re.fullmatch(r"[a-f0-9]{64}", obs["catalog_sha256"])):
            raise ValueError()
        result.update(status="OBSERVED_NOT_APPROVED", tls="VERIFIED", public_catalog="OBSERVED",
                      run_id=run_id, receipt_sha256=sha256(raw).hexdigest(),
                      public_table_count=len(obs["public_tables"]), observed_at=obs["observed_at"])
    except (OSError, ValueError, KeyError, TypeError, OverflowError):
        pass
    return result
