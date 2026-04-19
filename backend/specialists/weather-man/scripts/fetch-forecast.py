#!/usr/bin/env python3
# fetch-forecast.py
# Fetches a 3-day weather forecast for a given location.
# Usage: python fetch-forecast.py <location>

import sys
import json
import urllib.request
import urllib.parse

def fetch_forecast(location: str) -> dict:
    encoded = urllib.parse.quote(location)
    url = f"https://wttr.in/{encoded}?format=j1"
    req = urllib.request.Request(url, headers={"User-Agent": "weather-man-plugin/1.0"})
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
        "location": f"{area_name}, {country}",
        "forecast_days": days,
    }

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(json.dumps({"error": "Usage: fetch-forecast.py <location>"}))
        sys.exit(1)

    location = " ".join(sys.argv[1:])
    try:
        result = fetch_forecast(location)
        print(json.dumps(result, indent=2))
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(1)
