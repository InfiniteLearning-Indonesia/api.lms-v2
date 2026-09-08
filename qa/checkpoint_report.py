"""Read retained CP11/CP12 evidence; never execute database or publish commands.

Statement coverage is a union of identical source blocks, never an average of
percentages. Missing, stale, failed or duplicate evidence cannot close a slice.
"""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import uuid
from qa.evidence import source_fingerprint, verify_go_cases, write_new_json
from qa.run import ROOT


def exact_cases(expected, cases, key="id", passed="PASS"):
    ids = [case[key] for case in cases]
    return bool(expected) and len(expected) == len(set(expected)) and len(ids) == len(set(ids)) and set(ids) == set(expected) and all(case["status"] == passed for case in cases)


def union_coverage(profiles):
    blocks = {}
    for text in profiles:
        lines = text.splitlines()
        if not lines or lines[0] not in {"mode: set", "mode: count", "mode: atomic"}:
            raise ValueError("invalid coverage header")
        for line in lines[1:]:
            location, statements, count = line.rsplit(" ", 2)
            statements, count = int(statements), int(count)
            if statements < 0 or count < 0:
                raise ValueError("negative coverage counter")
            old = blocks.get(location)
            if old and old[0] != statements:
                raise ValueError("incompatible source blocks")
            blocks[location] = (statements, bool(count) or bool(old and old[1]))
    if not blocks:
        raise ValueError("empty coverage")
    packages = {}
    for location, (statements, executed) in blocks.items():
        package = location.rsplit("/", 1)[0]
        counts = packages.setdefault(package, {"covered": 0, "statements": 0})
        counts["statements"] += statements
        counts["covered"] += statements if executed else 0
    for counts in packages.values():
        counts["percent"] = round(100 * counts["covered"] / counts["statements"], 2) if counts["statements"] else None
    return packages


def deliverables(manifest, checks, current):
    return {name: {"status": "VERIFIED_LOCAL" if current and all(checks.get(k) is True for k in required) else "PENDING",
                   "required_checks": required}
            for name, required in manifest["deliverables"].items()}


def build_report(lab_dir, inventory_dir, default_dir, linux_dir):
    source = source_fingerprint(ROOT)
    load = lambda path: json.loads(path.read_text(encoding="utf-8"))
    lab, inventory, default, linux = [load(p / "summary.json") for p in (lab_dir, inventory_dir, default_dir, linux_dir)]
    current = all(r.get("source_hash") == source for r in (lab, inventory, default, linux))
    manifest = load(ROOT / "qa/manifests/CP11-CP12.json")
    old = load(ROOT / "qa/manifests/CP08-CP10.json")
    go = load(ROOT / "qa/manifests/go-tests.json")
    checks = {}
    default_ok = True
    for package, directory, result in (("M00", inventory_dir, inventory), ("M03", default_dir, default)):
        expected = load(ROOT / ("qa/manifests/" + package + ".json"))
        default_ok &= result.get("package") == package and result.get("automated_checks") == "PASS" and result.get("tested_source_hash") == source
        default_ok &= exact_cases(expected["expected_checks"], result["cases"])
        default_ok &= verify_go_cases(go["expected_tests"], load(directory / "go-cases.json")["cases"])
    checks["default_regression"] = bool(default_ok and linux.get("status") == "PASS" and linux.get("race") == "PASS" and linux.get("go_cases") == len(go["expected_tests"]))
    cp10 = load(lab_dir / "cp10-cases.json")
    checks["cp10_regression"] = (lab.get("status") == "PASS_SYNTHETIC_ONLY" and exact_cases(old["cp10_python"], cp10["cases"]) and cp10["exit_status"] == 0
                                 and all(verify_go_cases(old[cp+"_go"], load(lab_dir / (cp+"-cases.json"))) for cp in ("cp08", "cp09")))
    for cp in ("cp11", "cp12"):
        checks[cp+"_go"] = verify_go_cases(manifest[cp+"_go"], load(lab_dir / (cp+"-cases.json")))
        api = load(lab_dir / (cp+"-api-cases.json"))
        api_ok = api["exit_status"] == 0 and exact_cases(manifest[cp+"_python"], api["cases"]) and set(api["collected_ids"]) == set(manifest[cp+"_python"])
        checks[cp+"_regression"] = bool(checks[cp+"_go"] and api_ok)
        for case in api["cases"]:
            checks[case["id"]] = checks.get(case["id"], True) and api_ok and case["status"] == "PASS"
    current &= lab.get("lifecycle_reconciliation") == "PASS" and all(checks.values())
    ds = deliverables(manifest, checks, current)
    profiles = [default_dir / "go-coverage.out"] + [lab_dir / name for name in ("integration-coverage.out", "cp08-coverage.out", "cp09-coverage.out", "cp10-coverage.out", "cp11-coverage.out", "cp12-coverage.out", "cp11-api-coverage.out", "cp12-api-coverage.out")]
    coverage = union_coverage([p.read_text() for p in profiles]) if current else {}
    totals = {}
    for cp in ("CP11", "CP12"):
        entries = [v for k,v in ds.items() if k.startswith(cp+"-")]
        verified = sum(v["status"] == "VERIFIED_LOCAL" for v in entries)
        totals[cp] = dict(verified=verified, total=len(entries), percent=100*verified/len(entries))
    return dict(source_hash=source, status="VERIFIED_LOCAL_SLICES" if current else "PENDING", scope="LOCAL_SYNTHETIC_ONLY",
                input_runs=[r["run_id"] for r in (lab,inventory,default,linux)], checks=checks, deliverables=ds, deliverable_coverage=totals,
                statement_coverage_union=coverage, coverage_profiles=[str(p.relative_to(ROOT)) for p in profiles],
                full_M04_M05_verified=False, real_legacy_issuer="NOT_VERIFIED", staging_production_access="NONE",
                tagged_integration_race_detector="NOT_RUN", integration_lock_barriers="PASS" if checks["cp12_go"] else "NOT_VERIFIED")


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ("lab", "inventory", "default", "linux"):
        parser.add_argument("--"+name+"-run",required=True)
    args=parser.parse_args()
    parent=ROOT/".qa-artifacts"
    directories=[]
    for name in (args.lab_run,args.inventory_run,args.default_run,args.linux_run):
        if Path(name).name!=name:
            raise ValueError("run ID must be a basename")
        directory=(parent/name).resolve(strict=True)
        if directory.parent!=parent.resolve():
            raise ValueError("evidence escaped local artifact directory")
        directories.append(directory)
    result=build_report(*directories)
    result["run_id"]=datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")+"-cp11-cp12-report-"+uuid.uuid4().hex[:12]
    directory=parent/result["run_id"];directory.mkdir()
    write_new_json(directory/"summary.json",result)
    print(json.dumps({k:result[k] for k in ("run_id","source_hash","status","deliverable_coverage","statement_coverage_union")}))
    return 0 if result["status"]=="VERIFIED_LOCAL_SLICES" else 1


if __name__=="__main__":
    raise SystemExit(main())
