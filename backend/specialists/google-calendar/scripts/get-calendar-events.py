#!/usr/bin/env python3
import json
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Optional

ACCESS_TOKEN = "__TOKEN_access_token:string__"
CALENDAR_ID = "__TOKEN_calendar_id:string__"
TIME_MIN = "__TOKEN_time_min:string__"
TIME_MAX = "__TOKEN_time_max:string__"
MAX_RESULTS = "__TOKEN_max_results:string__"
SCRIPT_ID = "__SCRIPT_ID__"
TTY = "/dev/ttyAMA0"


def emit(obj: dict) -> None:
    with open(TTY, "w") as f:
        f.write(json.dumps(obj) + "\n")
        f.flush()


def is_unresolved(value: str) -> bool:
    return value.startswith("__TOKEN_")


def clean_optional(value: str, default: Optional[str] = None) -> Optional[str]:
    if is_unresolved(value):
        return default
    cleaned = value.strip()
    if not cleaned:
        return default
    return cleaned


def normalize_access_token(value: str) -> str:
    token = value.strip()
    if token.lower().startswith("bearer "):
        token = token[7:].strip()
    if not token:
        raise ValueError("access_token is empty")
    if token.count(".") == 2:
        raise ValueError(
            "access_token appears to be a JWT/ID token. Store an OAuth 2.0 access token with calendar scope instead"
        )
    return token


def format_google_api_error(code: int, details: str) -> dict:
    payload = {
        "error": f"google calendar API error: {code}",
        "details": details,
    }
    if code == 401:
        payload["hint"] = (
            "Invalid credentials. Ensure the stored secret is a valid OAuth 2.0 access token "
            "(not an ID token/API key), includes calendar scope, and is not expired."
        )
    if code == 403:
        payload["hint"] = (
            "Permission denied. Ensure the token has Google Calendar permissions "
            "and the target calendar is readable by that account."
        )
    return payload


def parse_max_results(value: str) -> int:
    cleaned = clean_optional(value, "50")
    try:
        parsed = int(cleaned if cleaned is not None else "50")
    except ValueError as exc:
        raise ValueError("max_results must be an integer") from exc

    if parsed < 1:
        raise ValueError("max_results must be >= 1")
    if parsed > 250:
        return 250
    return parsed


def get_events(access_token: str, calendar_id: str, time_min: Optional[str], time_max: Optional[str], max_results: int) -> dict:
    encoded_calendar = urllib.parse.quote(calendar_id, safe="")
    params = {
        "singleEvents": "true",
        "orderBy": "startTime",
        "maxResults": str(max_results),
    }
    if time_min:
        params["timeMin"] = time_min
    if time_max:
        params["timeMax"] = time_max

    url = "https://www.googleapis.com/calendar/v3/calendars/{}/events?{}".format(
        encoded_calendar,
        urllib.parse.urlencode(params),
    )
    req = urllib.request.Request(
        url,
        headers={
            "Authorization": f"Bearer {access_token}",
            "Accept": "application/json",
            "User-Agent": "smcp-google-calendar/1.0",
        },
    )

    with urllib.request.urlopen(req, timeout=20) as resp:
        payload = json.loads(resp.read().decode())

    events = []
    for item in payload.get("items", []):
        events.append(
            {
                "id": item.get("id"),
                "summary": item.get("summary", "(untitled)"),
                "status": item.get("status"),
                "start": item.get("start", {}).get("dateTime") or item.get("start", {}).get("date"),
                "end": item.get("end", {}).get("dateTime") or item.get("end", {}).get("date"),
                "location": item.get("location"),
                "description": item.get("description"),
                "html_link": item.get("htmlLink"),
            }
        )

    return {
        "calendar_id": calendar_id,
        "count": len(events),
        "events": events,
    }


if __name__ == "__main__":
    if is_unresolved(ACCESS_TOKEN):
        emit(
            {
                "type": "result",
                "script": SCRIPT_ID,
                "ok": False,
                "output": json.dumps({"error": "access_token token was not replaced"}),
            }
        )
        sys.exit(1)

    try:
        calendar_id = clean_optional(CALENDAR_ID, "primary")
        time_min = clean_optional(TIME_MIN)
        time_max = clean_optional(TIME_MAX)
        max_results = parse_max_results(MAX_RESULTS)

        if calendar_id is None:
            calendar_id = "primary"

        access_token = normalize_access_token(ACCESS_TOKEN)
        data = get_events(access_token, calendar_id, time_min, time_max, max_results)
        emit({"type": "result", "script": SCRIPT_ID, "ok": True, "output": json.dumps(data)})
    except urllib.error.HTTPError as e:
        details = e.read().decode("utf-8", errors="replace")
        emit(
            {
                "type": "result",
                "script": SCRIPT_ID,
                "ok": False,
                "output": json.dumps(format_google_api_error(e.code, details)),
            }
        )
        sys.exit(1)
    except Exception as e:
        emit(
            {
                "type": "result",
                "script": SCRIPT_ID,
                "ok": False,
                "output": json.dumps({"error": str(e)}),
            }
        )
        sys.exit(1)
