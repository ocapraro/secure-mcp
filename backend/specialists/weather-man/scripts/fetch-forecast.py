#!/usr/bin/env python3
# fetch-forecast.py

import json
import sys
import time
import urllib.request
import urllib.parse
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


def fetch_forecast(location: str) -> dict:
    encoded = urllib.parse.quote(location)
    url = f"https://wttr.in/{encoded}?format=j1"
    req = urllib.request.Request(
        url,
        headers={"User-Agent": "weather-man-plugin/1.0"}
    )

    with urllib.request.urlopen(req, timeout=10) as resp:
        data = json.loads(resp.read().decode())

    nearest = data["nearest_area"][0]
    area_name = nearest["areaName"][0]["value"]
    country = nearest["country"][0]["value"]

    days = []
    for day in data["weather"]:
        hourly_summary = []
        for h in day["hourly"]:
            hourly_summary.append({
                "time": h["time"],
                "temp_c": h["tempC"],
                "temp_f": h["tempF"],
                "feels_like_c": h["FeelsLikeC"],
                "feels_like_f": h["FeelsLikeF"],
                "chance_of_rain_pct": h["chanceofrain"],
                "chance_of_snow_pct": h["chanceofsnow"],
                "wind_speed_kmph": h["windspeedKmph"],
                "description": h["weatherDesc"][0]["value"],
            })

        days.append({
            "date": day["date"],
            "max_temp_c": day["maxtempC"],
            "max_temp_f": day["maxtempF"],
            "min_temp_c": day["mintempC"],
            "min_temp_f": day["mintempF"],
            "avg_humidity_pct": day["hourly"][4]["humidity"] if len(day["hourly"]) > 4 else None,
            "sunrise": day["astronomy"][0]["sunrise"],
            "sunset": day["astronomy"][0]["sunset"],
            "hourly": hourly_summary,
        })

    return {
        "type": "forecast_result",
        "location": f"{area_name}, {country}",
        "forecast_days": days,
        "ok": True,
    }


if __name__ == "__main__":
    if LOCATION.startswith("__TOKEN_"):
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": "location token was not replaced"})})
        sys.exit(1)

    try:
        emit({"type": "status", "msg": "waiting for network"})
        wait_for_network()

        result = fetch_forecast(LOCATION)
        emit({"type": "result", "script": SCRIPT_ID, "ok": True, "output": json.dumps(result)})

    except Exception as e:
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": str(e)})})
        sys.exit(1)