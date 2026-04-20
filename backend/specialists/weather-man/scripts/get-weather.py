#!/usr/bin/env python3
import json
import sys
import time
import urllib.parse
import urllib.request
import subprocess

LOCATION = "__TOKEN_location:string__"
SCRIPT_ID = "__SCRIPT_ID__"
TTY = "/dev/ttyAMA0"


def emit(obj: dict) -> None:
    with open(TTY, "w") as f:
        f.write(json.dumps(obj) + "\n")
        f.flush()


def wait_for_network(timeout: int = 15) -> None:
    for _ in range(timeout):
        has_ipv4 = subprocess.run(
            ["sh", "-c", "ip -4 addr show eth0 | grep -q 'inet '"]
        ).returncode == 0

        has_route = subprocess.run(
            ["sh", "-c", "ip route | grep -q '^default '"]
        ).returncode == 0

        if has_ipv4 and has_route:
            return

        time.sleep(1)

    raise RuntimeError("network was not ready in time")


def get_weather(location: str) -> dict:
    encoded = urllib.parse.quote(location)
    url = f"https://wttr.in/{encoded}?format=j1"
    req = urllib.request.Request(
        url,
        headers={"User-Agent": "weather-man-plugin/1.0"},
    )

    with urllib.request.urlopen(req, timeout=10) as resp:
        data = json.loads(resp.read().decode())

    current = data["current_condition"][0]
    nearest = data["nearest_area"][0]

    return {
        "location": f'{nearest["areaName"][0]["value"]}, {nearest["country"][0]["value"]}',
        "temp_c": current["temp_C"],
        "temp_f": current["temp_F"],
        "feels_like_c": current["FeelsLikeC"],
        "feels_like_f": current["FeelsLikeF"],
        "humidity_pct": current["humidity"],
        "wind_speed_kmph": current["windspeedKmph"],
        "wind_direction": current["winddir16Point"],
        "visibility_km": current["visibility"],
        "description": current["weatherDesc"][0]["value"],
        "uv_index": current["uvIndex"],
    }


if __name__ == "__main__":
    if LOCATION.startswith("__TOKEN_"):
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": "{\"error\": \"location token was not replaced\"}"})
        sys.exit(1)

    try:
        wait_for_network()
        data = get_weather(LOCATION)
        emit({"type": "result", "script": SCRIPT_ID, "ok": True, "output": json.dumps(data)})
    except Exception as e:
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": str(e)})})
        sys.exit(1)