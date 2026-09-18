"""Fail-closed module -> checkpoint -> case -> deliverable evidence adapter."""
from pathlib import Path
import json


def _unique_object(pairs):
    value = {}
    for key, item in pairs:
        if key in value:
            raise ValueError("duplicate JSON key")
        value[key] = item
    return value


def load(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_unique_object)


def validate(registry: dict, deliverables: set[str], cases: set[str]) -> None:
    if registry.get("version") != 1 or registry.get("scope") != "LOCAL_SYNTHETIC_SLICE":
        raise ValueError("registry scope rejected")
    checkpoints = registry.get("checkpoints")
    if not isinstance(checkpoints, list) or not checkpoints:
        raise ValueError("checkpoint registry empty")
    checkpoint_ids = [item.get("id") for item in checkpoints]
    if len(checkpoint_ids) != len(set(checkpoint_ids)) or any(not str(value).startswith(registry.get("module", "M03") + "-CP") for value in checkpoint_ids):
        raise ValueError("duplicate or foreign checkpoint")
    for checkpoint in checkpoints:
        mapped_deliverables = checkpoint.get("deliverables", [])
        mapped_cases = checkpoint.get("cases", [])
        if (not mapped_deliverables or len(mapped_deliverables) != len(set(mapped_deliverables)) or
                set(mapped_deliverables) - deliverables):
            raise ValueError("invalid checkpoint deliverables")
        if not mapped_cases or len(mapped_cases) != len(set(mapped_cases)) or set(mapped_cases) - cases:
            raise ValueError("invalid checkpoint cases")


def reconcile(registry: dict, checkpoint_id: str, evidence: list[dict], source_hash: str, tested_hash: str) -> dict:
    checkpoint = next((item for item in registry.get("checkpoints", []) if item.get("id") == checkpoint_id), None)
    if checkpoint is None:
        raise ValueError("unknown checkpoint")
    ids = [item.get("id") for item in evidence]
    expected = checkpoint["cases"]
    if len(ids) != len(set(ids)):
        raise ValueError("duplicate checkpoint evidence")
    missing = sorted(set(expected) - set(ids))
    unexpected = sorted(set(ids) - set(expected))
    current = bool(source_hash) and source_hash == tested_hash
    passed = not missing and not unexpected and current and all(item.get("status") == "PASS" for item in evidence)
    return {"checkpoint": checkpoint_id, "status": "PASS" if passed else "BLOCKED",
            "current": current, "missing": missing, "unexpected": unexpected}
