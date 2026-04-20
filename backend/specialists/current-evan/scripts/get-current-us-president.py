#!/usr/bin/env python3
import json
import sys
from datetime import date

SCRIPT_ID = "__SCRIPT_ID__"
TTY = "/dev/ttyAMA0"


def emit(obj: dict) -> None:
    with open(TTY, "w") as f:
        f.write(json.dumps(obj) + "\n")
        f.flush()


def resolve_current_president(today: date) -> dict:
    administrations = [
        {
            "term_start": date(2009, 1, 20),
            "president": "Barack Obama",
            "vice_president": "Joe Biden",
            "party": "Democratic",
        },
        {
            "term_start": date(2017, 1, 20),
            "president": "Donald Trump",
            "vice_president": "Mike Pence",
            "party": "Republican",
        },
        {
            "term_start": date(2021, 1, 20),
            "president": "Joe Biden",
            "vice_president": "Kamala Harris",
            "party": "Democratic",
        },
        {
            "term_start": date(2025, 1, 20),
            "president": "Donald Trump",
            "vice_president": "JD Vance",
            "party": "Republican",
        },
    ]

    current = administrations[0]
    for admin in administrations:
        if today >= admin["term_start"]:
            current = admin
        else:
            break

    return {
        "date": today.isoformat(),
        "president": current["president"],
        "vice_president": current["vice_president"],
        "party": current["party"],
        "term_start": current["term_start"].isoformat(),
    }


if __name__ == "__main__":
    try:
        result = resolve_current_president(date.today())
        emit({"type": "result", "script": SCRIPT_ID, "ok": True, "output": json.dumps(result)})
    except Exception as e:
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": str(e)})})
        sys.exit(1)
