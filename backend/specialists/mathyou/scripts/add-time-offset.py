#!/usr/bin/env python3
import json
import sys
from datetime import datetime, timedelta

DATETIME_INPUT = "__TOKEN_datetime_input:string__"
DAYS = __TOKEN_days:int__
HOURS = __TOKEN_hours:int__
MINUTES = __TOKEN_minutes:int__
SCRIPT_ID = "__SCRIPT_ID__"
TTY = "/dev/ttyAMA0"


def emit(obj: dict) -> None:
    with open(TTY, "w") as f:
        f.write(json.dumps(obj) + "\n")
        f.flush()


def parse_datetime(value: str) -> datetime:
    text = value.strip()
    if not text:
        raise ValueError("datetime_input must not be empty")
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"

    if "T" in text or ":" in text:
        return datetime.fromisoformat(text)

    return datetime.fromisoformat(text + "T00:00:00")


if __name__ == "__main__":
    if DATETIME_INPUT.startswith("__TOKEN_"):
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": "datetime_input token was not replaced"})})
        sys.exit(1)

    try:
        start = parse_datetime(DATETIME_INPUT)
        out = start + timedelta(days=DAYS, hours=HOURS, minutes=MINUTES)

        result = {
            "input_datetime": DATETIME_INPUT.strip(),
            "offset": {
                "days": DAYS,
                "hours": HOURS,
                "minutes": MINUTES,
            },
            "result_iso": out.isoformat(),
            "result_date": out.strftime("%Y-%m-%d"),
            "result_time": out.strftime("%H:%M:%S"),
        }
        emit({"type": "result", "script": SCRIPT_ID, "ok": True, "output": json.dumps(result)})
    except Exception as e:
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": str(e)})})
        sys.exit(1)
