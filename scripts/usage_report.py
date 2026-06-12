#!/usr/bin/env python3
import argparse
import json
import sys
from collections import OrderedDict
from pathlib import Path


def parse_args():
    parser = argparse.ArgumentParser(description="Summarize baixin-switch usage JSONL logs")
    parser.add_argument("--file", default="./logs/usage.jsonl", help="usage JSONL path")
    parser.add_argument("--by", choices=["day", "week", "session"], help="aggregation dimension")
    parser.add_argument("--session", help="show one session timeline")
    parser.add_argument("--from", dest="date_from", help="inclusive date filter YYYY-MM-DD")
    parser.add_argument("--to", dest="date_to", help="inclusive date filter YYYY-MM-DD")
    parser.add_argument("--json", action="store_true", help="emit JSON")
    args = parser.parse_args()
    if not args.by and not args.session:
        parser.error("one of --by or --session is required")
    return args


def iter_events(path: Path):
    if not path.exists():
        return
    with path.open("r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                yield json.loads(line)
            except json.JSONDecodeError:
                continue


def in_range(event, date_from, date_to):
    date = str(event.get("date") or "")[:10]
    if date_from and date < date_from:
        return False
    if date_to and date > date_to:
        return False
    return True


def aggregate(events, dimension):
    rows = OrderedDict()
    for event in events:
        key = str(event.get(key_field(dimension)) or "")
        if not key:
            key = "unknown"
        if key not in rows:
            rows[key] = base_row(dimension, key)
        row = rows[key]
        row["requests"] += 1
        row["input_tokens"] += int(event.get("input_tokens") or 0)
        row["output_tokens"] += int(event.get("output_tokens") or 0)
        row["total_tokens"] += int(event.get("total_tokens") or 0)
        ts = str(event.get("ts") or "")
        if dimension == "session":
            if not row["first_seen"] or ts < row["first_seen"]:
                row["first_seen"] = ts
            if not row["last_seen"] or ts > row["last_seen"]:
                row["last_seen"] = ts
    return list(rows.values())


def key_field(dimension):
    if dimension == "day":
        return "date"
    if dimension == "week":
        return "week"
    return "session_id"


def base_row(dimension, key):
    row = {
        dimension_key(dimension): key,
        "requests": 0,
        "input_tokens": 0,
        "output_tokens": 0,
        "total_tokens": 0,
    }
    if dimension == "session":
        row["first_seen"] = ""
        row["last_seen"] = ""
    return row


def dimension_key(dimension):
    if dimension == "day":
        return "date"
    if dimension == "week":
        return "week"
    return "session_id"


def timeline(events, session_id):
    out = []
    for event in events:
        if str(event.get("session_id") or "") != session_id:
            continue
        out.append({
            "ts": event.get("ts", ""),
            "request_id": event.get("request_id", ""),
            "model": event.get("model", ""),
            "status": event.get("status", 0),
            "input_tokens": int(event.get("input_tokens") or 0),
            "output_tokens": int(event.get("output_tokens") or 0),
            "total_tokens": int(event.get("total_tokens") or 0),
            "request_preview": event.get("request_preview", ""),
            "response_preview": event.get("response_preview", ""),
        })
    out.sort(key=lambda item: item["ts"])
    return out


def print_table(rows, dimension):
    if dimension == "session":
        headers = ["session_id", "requests", "input_tokens", "output_tokens", "total_tokens", "first_seen", "last_seen"]
    else:
        headers = [dimension_key(dimension), "requests", "input_tokens", "output_tokens", "total_tokens"]
    print_rows(rows, headers)


def print_timeline(rows):
    headers = ["ts", "request_id", "model", "status", "input_tokens", "output_tokens", "total_tokens", "response_preview"]
    print_rows(rows, headers)


def print_rows(rows, headers):
    widths = {h: len(h) for h in headers}
    for row in rows:
        for h in headers:
            widths[h] = max(widths[h], len(str(row.get(h, ""))))
    print("  ".join(h.ljust(widths[h]) for h in headers))
    for row in rows:
        print("  ".join(str(row.get(h, "")).ljust(widths[h]) for h in headers))


def main():
    args = parse_args()
    events = [event for event in iter_events(Path(args.file)) if in_range(event, args.date_from, args.date_to)]
    if args.session:
        rows = timeline(events, args.session)
        if args.json:
            print(json.dumps(rows, ensure_ascii=False, indent=2))
        else:
            print_timeline(rows)
        return
    rows = aggregate(events, args.by)
    if args.json:
        print(json.dumps(rows, ensure_ascii=False, indent=2))
    else:
        print_table(rows, args.by)


if __name__ == "__main__":
    try:
        main()
    except BrokenPipeError:
        sys.exit(0)
