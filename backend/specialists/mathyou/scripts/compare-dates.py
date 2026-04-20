#!/usr/bin/env python3
import json
import sys
from datetime import datetime

DATE_A = "__TOKEN_date_a:string__"
DATE_B = "__TOKEN_date_b:string__"
SCRIPT_ID = "__SCRIPT_ID__"
TTY = "/dev/ttyAMA0"


def emit(obj: dict) -> None:
    with open(TTY, "w") as f:
        f.write(json.dumps(obj) + "\n")
        f.flush()


def is_unresolved(value: str) -> bool:
    return value.startswith("__TOKEN_")


def parse_iso(value: str) -> datetime:
    text = value.strip()
    if not text:
        raise ValueError("date value must not be empty")
    # Support trailing Z as UTC.
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"

    if "T" in text or ":" in text:
        return datetime.fromisoformat(text)

    return datetime.fromisoformat(text + "T00:00:00")


if __name__ == "__main__":
    if is_unresolved(DATE_A) or is_unresolved(DATE_B):
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": "date tokens were not replaced"})})
        sys.exit(1)

    try:
        a = parse_iso(DATE_A)
        b = parse_iso(DATE_B)
        delta_seconds = (b - a).total_seconds()

        if delta_seconds > 0:
            relation = "date_b_is_later"
        elif delta_seconds < 0:
            relation = "date_a_is_later"
        else:
            relation = "equal"

        result = {
            "date_a": DATE_A.strip(),
            "date_b": DATE_B.strip(),
            "relation": relation,
            "difference_days": delta_seconds / 86400.0,
            "difference_hours": delta_seconds / 3600.0,
            "difference_seconds": delta_seconds,
        }

        emit({"type": "result", "script": SCRIPT_ID, "ok": True, "output": json.dumps(result)})
    except Exception as e:
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": str(e)})})
        sys.exit(1)
