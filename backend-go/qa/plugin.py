"""Minimal Pytest evidence plugin. Never captures credentials or assertion text."""
import os
from pathlib import Path
import pytest
from qa.evidence import write_new_json

_node_ids: dict[str, str] = {}
_results: dict[str, dict] = {}


def pytest_sessionstart(session):
    _node_ids.clear()
    _results.clear()


def pytest_collection_modifyitems(items):
    for item in items:
        marker = item.get_closest_marker("case")
        if not marker or len(marker.args) != 1 or not isinstance(marker.args[0], str):
            raise pytest.UsageError("each QA test requires one stable case ID")
        case_id = marker.args[0]
        if case_id in _node_ids.values():
            raise pytest.UsageError("duplicate QA case ID")
        _node_ids[item.nodeid] = case_id


def pytest_runtest_logreport(report):
    case_id = _node_ids.get(report.nodeid)
    if case_id is None:
        return
    result = _results.setdefault(case_id, {"id": case_id, "status": "NOT_RUN", "duration": 0})
    result["duration"] += report.duration
    if report.when == "call" and report.passed and hasattr(report, "wasxfail"):
        result["status"] = "FAIL"  # Unexpected pass is not accepted evidence.
    elif report.failed:
        result["status"] = "FAIL" if report.when == "call" else "ERROR"
    elif report.skipped and result["status"] not in {"FAIL", "ERROR"}:
        result["status"] = "SKIPPED"
    elif report.when == "call" and report.passed and result["status"] == "NOT_RUN":
        result["status"] = "PASS"


def pytest_sessionfinish(session, exitstatus):
    destination = os.environ.get("QA_CASE_RESULTS")
    if destination:
        write_new_json(Path(destination), {"exit_status": int(exitstatus),
                                          "collected_ids": sorted(_node_ids.values()),
                                          "cases": sorted(_results.values(), key=lambda r: r["id"])})
