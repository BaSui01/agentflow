#!/usr/bin/env python3
import json
import re
import subprocess
from datetime import datetime
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
EVIDENCE_DIR = ROOT / "docs" / "重构计划" / "evidence"
EVIDENCE_DIR.mkdir(parents=True, exist_ok=True)


def run(cmd):
    p = subprocess.run(
        cmd,
        cwd=ROOT,
        capture_output=True,
        text=True,
        shell=True,
        encoding="utf-8",
    )
    return p.returncode, p.stdout + p.stderr


def main():
    ts = datetime.now().strftime("%Y-%m-%dT%H:%M:%S")

    code_metrics, out_metrics = run("go test -run TestRAGBaselineMetrics -v ./rag -count=1")
    if code_metrics != 0:
        raise SystemExit(out_metrics)

    m = re.search(
        r"BASELINE_METRICS latency_ms=([0-9.]+) recall_at_k=([0-9.]+) mrr=([0-9.]+) error_rate=([0-9.]+)",
        out_metrics,
    )
    if not m:
        raise SystemExit("failed to parse BASELINE_METRICS from test output")

    metrics = {
        "latency_ms": float(m.group(1)),
        "recall_at_k": float(m.group(2)),
        "mrr": float(m.group(3)),
        "error_rate": float(m.group(4)),
    }

    code_suite, out_suite = run("go test ./rag/... -count=1")
    if code_suite != 0:
        raise SystemExit(out_suite)

    payload = {
        "captured_at": ts,
        "metrics": metrics,
        "commands": {
            "metrics_test": "go test -run TestRAGBaselineMetrics -v ./rag -count=1",
            "suite_test": "go test ./rag/... -count=1",
        },
    }

    out_file = EVIDENCE_DIR / "rag_baseline_metrics_latest.json"
    out_file.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"[OK] baseline captured: {out_file}")
    print(json.dumps(metrics, ensure_ascii=False))


if __name__ == "__main__":
    main()
