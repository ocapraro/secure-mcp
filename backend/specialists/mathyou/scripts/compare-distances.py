#!/usr/bin/env python3
import json
import sys

VALUE_A = "__TOKEN_value_a:string__"
UNIT_A = "__TOKEN_unit_a:string__"
VALUE_B = "__TOKEN_value_b:string__"
UNIT_B = "__TOKEN_unit_b:string__"
SCRIPT_ID = "__SCRIPT_ID__"
TTY = "/dev/ttyAMA0"

UNITS_TO_METERS = {
    "mm": 0.001,
    "cm": 0.01,
    "m": 1.0,
    "km": 1000.0,
    "in": 0.0254,
    "ft": 0.3048,
    "yd": 0.9144,
    "mi": 1609.344,
}


def emit(obj: dict) -> None:
    with open(TTY, "w") as f:
        f.write(json.dumps(obj) + "\n")
        f.flush()


def is_unresolved(value: str) -> bool:
    return value.startswith("__TOKEN_")


def to_meters(value_raw: str, unit_raw: str) -> float:
    value = float(value_raw.strip())
    unit = unit_raw.strip().lower()
    if unit not in UNITS_TO_METERS:
        raise ValueError(f"unsupported unit: {unit}")
    return value * UNITS_TO_METERS[unit]


if __name__ == "__main__":
    if any(is_unresolved(v) for v in [VALUE_A, UNIT_A, VALUE_B, UNIT_B]):
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": "distance tokens were not replaced"})})
        sys.exit(1)

    try:
        a_m = to_meters(VALUE_A, UNIT_A)
        b_m = to_meters(VALUE_B, UNIT_B)
        diff = a_m - b_m

        if diff > 0:
            relation = "a_is_greater"
            larger = "a"
            smaller = "b"
        elif diff < 0:
            relation = "b_is_greater"
            larger = "b"
            smaller = "a"
        else:
            relation = "equal"
            larger = "equal"
            smaller = "equal"

        result = {
            "input_a": {"value": VALUE_A.strip(), "unit": UNIT_A.strip().lower()},
            "input_b": {"value": VALUE_B.strip(), "unit": UNIT_B.strip().lower()},
            "a_meters": a_m,
            "b_meters": b_m,
            "relation": relation,
            "difference_meters": abs(diff),
            "larger": larger,
            "smaller": smaller,
        }

        emit({"type": "result", "script": SCRIPT_ID, "ok": True, "output": json.dumps(result)})
    except Exception as e:
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": str(e)})})
        sys.exit(1)
