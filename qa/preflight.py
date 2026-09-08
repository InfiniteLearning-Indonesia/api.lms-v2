"""Pure, fail-closed operator checks. This module never connects to a DB."""
from urllib.parse import urlsplit
import ipaddress


class GateBlocked(ValueError):
    pass


def require_local_api(base_url: str) -> None:
    try:
        parsed = urlsplit(base_url)
        address = ipaddress.ip_address(parsed.hostname or "")
        port = parsed.port
    except ValueError:
        raise GateBlocked("numeric loopback API address required") from None
    if (
        parsed.scheme != "http" or not address.is_loopback or not port
        or parsed.username or parsed.password or parsed.query or parsed.fragment
        or parsed.path not in ("", "/")
    ):
        raise GateBlocked("only explicit loopback synthetic API is permitted")


def require_distinct_verified_databases(source: dict, target: dict) -> None:
    """Validate a trusted operator receipt, NOT user-selected environment labels.

    Real server receipt acquisition/approval verification is not implemented yet;
    this predicate alone cannot authorize an import.
    """
    required = ("project_id", "database_id", "server_receipt_hash", "ca_fingerprint")
    for observation in (source, target):
        if observation.get("identity_verified") is not True:
            raise GateBlocked("database identity not verified")
        if any(not isinstance(observation.get(k), str) or not observation[k].strip() for k in required):
            raise GateBlocked("verified identity evidence incomplete")
        if observation.get("tls_mode") != "verify-full":
            raise GateBlocked("TLS peer and hostname verification required")
    if (source["project_id"], source["database_id"]) == (target["project_id"], target["database_id"]):
        raise GateBlocked("source and target identify the same database")
    if source.get("role_read_only") is not True:
        raise GateBlocked("source role must be verified read-only")


def require_backup_receipt(receipt: dict, operation_checksum: str) -> None:
    """Evidence shape check only; artifact/restore verification is an M01 task."""
    if not operation_checksum or receipt.get("operation_checksum") != operation_checksum:
        raise GateBlocked("backup scope/checksum mismatch")
    for key in ("backup_id", "artifact_sha256", "restore_evidence_id", "reviewer_reference"):
        if not receipt.get(key):
            raise GateBlocked("backup/restore evidence incomplete")
    if receipt.get("status") != "RESTORE_VERIFIED":
        raise GateBlocked("backup has not passed restore verification")


def require_handoff_evidence(receipt: dict) -> None:
    for key in ("owner", "reviewer", "source_schema_hash", "operation_checksum"):
        if not receipt.get(key):
            raise GateBlocked("operator handoff incomplete")
    if not receipt.get("allowed_operations"):
        raise GateBlocked("no approved operation scope")
