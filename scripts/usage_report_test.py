#!/usr/bin/env python3
import json
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "usage_report.py"


def write_jsonl(path: Path) -> None:
    rows = [
        {
            "ts": "2026-06-12T10:00:00+08:00",
            "date": "2026-06-12",
            "week": "2026-W24",
            "request_id": "req-1",
            "session_id": "sess-a",
            "model": "deepseek-v4-pro",
            "status": 200,
            "input_tokens": 10,
            "output_tokens": 5,
            "total_tokens": 15,
            "request_preview": "hello",
            "response_preview": "world",
        },
        {
            "ts": "2026-06-12T10:05:00+08:00",
            "date": "2026-06-12",
            "week": "2026-W24",
            "request_id": "req-2",
            "session_id": "sess-a",
            "model": "deepseek-v4-pro",
            "status": 200,
            "input_tokens": 7,
            "output_tokens": 3,
            "total_tokens": 10,
            "request_preview": "next",
            "response_preview": "answer",
        },
        {
            "ts": "2026-06-19T11:00:00+08:00",
            "date": "2026-06-19",
            "week": "2026-W25",
            "request_id": "req-3",
            "session_id": "sess-b",
            "model": "deepseek-v4-pro",
            "status": 200,
            "input_tokens": 20,
            "output_tokens": 8,
            "total_tokens": 28,
            "request_preview": "third",
            "response_preview": "reply",
        },
    ]
    with path.open("w", encoding="utf-8") as f:
        for row in rows:
            f.write(json.dumps(row, ensure_ascii=False) + "\n")


def run_report(*args: str) -> str:
    result = subprocess.run(
        [sys.executable, str(SCRIPT), *args],
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=True,
    )
    return result.stdout


def assert_equal(actual, expected, message: str) -> None:
    if actual != expected:
        raise AssertionError(f"{message}: expected {expected!r}, got {actual!r}")


def test_day_json(path: Path) -> None:
    data = json.loads(run_report("--file", str(path), "--by", "day", "--json"))
    assert_equal(data[0]["date"], "2026-06-12", "first day")
    assert_equal(data[0]["requests"], 2, "day requests")
    assert_equal(data[0]["total_tokens"], 25, "day tokens")
    assert_equal(data[1]["date"], "2026-06-19", "second day")


def test_week_json(path: Path) -> None:
    data = json.loads(run_report("--file", str(path), "--by", "week", "--json"))
    assert_equal(data[0]["week"], "2026-W24", "first week")
    assert_equal(data[0]["total_tokens"], 25, "week tokens")
    assert_equal(data[1]["week"], "2026-W25", "second week")


def test_session_json(path: Path) -> None:
    data = json.loads(run_report("--file", str(path), "--by", "session", "--json"))
    assert_equal(data[0]["session_id"], "sess-a", "first session")
    assert_equal(data[0]["requests"], 2, "session requests")
    assert_equal(data[0]["total_tokens"], 25, "session tokens")


def test_session_timeline(path: Path) -> None:
    data = json.loads(run_report("--file", str(path), "--session", "sess-a", "--json"))
    assert_equal(len(data), 2, "timeline length")
    assert_equal(data[0]["request_id"], "req-1", "first timeline request")
    assert_equal(data[1]["request_id"], "req-2", "second timeline request")


def main() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        path = Path(tmp) / "usage.jsonl"
        write_jsonl(path)
        test_day_json(path)
        test_week_json(path)
        test_session_json(path)
        test_session_timeline(path)


if __name__ == "__main__":
    main()
