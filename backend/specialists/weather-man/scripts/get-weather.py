#!/usr/bin/env python3
# get-weather.py
# Fetches current weather for a given location.
# Usage: python get-weather.py <location>

import sys
import json
import urllib.request
import urllib.parse

def get_weather(location: str) -> dict:
    encoded = urllib.parse.quote(location)
    url = f"https://wttr.in/{encoded}?format=j1"
    req = urllib.request.Request(url, headers={"User-Agent": "weather-man-plugin/1.0"})
    with urllib.request.urlopen(req, timeout=10) as resp:
        data = json.loads(resp.read().decode())

    current = data["current_condition"][0]
    nearest = data["nearest_area"][0]
    area_name = nearest["areaName"][0]["value"]
    country = nearest["country"][0]["value"]

    return {
        "location": f"{area_name}, {country}",
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
    if len(sys.argv) < 2:
        print(json.dumps({"error": "Usage: get-weather.py <location>"}))
        sys.exit(1)

    location = " ".join(sys.argv[1:])
    try:
        result = get_weather(location)
        print(json.dumps(result, indent=2))
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(1)
