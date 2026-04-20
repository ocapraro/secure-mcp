#!/usr/bin/env python3
import json
import sys
from datetime import datetime, timedelta

DATE_INPUT = "__TOKEN_date_input:string__"
DAYS = __TOKEN_days:int__
SCRIPT_ID = "__SCRIPT_ID__"
TTY = "/dev/ttyAMA0"


def emit(obj: dict) -> None:
    with open(TTY, "w") as f:
        f.write(json.dumps(obj) + "\n")
        f.flush()


def parse_date(value: str) -> datetime:
    text = value.strip()
    if not text:
        raise ValueError("date_input must not be empty")
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"

    if "T" in text:
        dt = datetime.fromisoformat(text)
        return dt

    return datetime.fromisoformat(text + "T00:00:00")


if __name__ == "__main__":
    if DATE_INPUT.startswith("__TOKEN_"):
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": "date_input token was not replaced"})})
        sys.exit(1)

    try:
        start = parse_date(DATE_INPUT)
        out = start + timedelta(days=DAYS)

        result = {
            "input_date": DATE_INPUT.strip(),
            "days_added": DAYS,
            "result_iso": out.isoformat(),
            "result_date": out.strftime("%Y-%m-%d"),
        }
        emit({"type": "result", "script": SCRIPT_ID, "ok": True, "output": json.dumps(result)})
    except Exception as e:
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": str(e)})})
        sys.exit(1)
