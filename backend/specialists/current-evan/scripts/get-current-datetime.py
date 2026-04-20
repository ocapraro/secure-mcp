#!/usr/bin/env python3
import json
import sys
from datetime import datetime

SCRIPT_ID = "__SCRIPT_ID__"
TTY = "/dev/ttyAMA0"


def emit(obj: dict) -> None:
    with open(TTY, "w") as f:
        f.write(json.dumps(obj) + "\n")
        f.flush()


if __name__ == "__main__":
    try:
        now = datetime.now().astimezone()
        result = {
            "iso_datetime": now.isoformat(),
            "iso_date": now.strftime("%Y-%m-%d"),
            "time_24h": now.strftime("%H:%M:%S"),
            "weekday": now.strftime("%A"),
            "timezone": now.tzname() or "UTC",
            "unix_timestamp": int(now.timestamp()),
        }
        emit({"type": "result", "script": SCRIPT_ID, "ok": True, "output": json.dumps(result)})
    except Exception as e:
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": str(e)})})
        sys.exit(1)
