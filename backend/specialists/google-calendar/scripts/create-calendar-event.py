#!/usr/bin/env python3
import json
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Optional

ACCESS_TOKEN = "__TOKEN_access_token:string__"
SUMMARY = "__TOKEN_summary:string__"
START_TIME = "__TOKEN_start_time:string__"
END_TIME = "__TOKEN_end_time:string__"
CALENDAR_ID = "__TOKEN_calendar_id:string__"
TIMEZONE = "__TOKEN_timezone:string__"
DESCRIPTION = "__TOKEN_description:string__"
LOCATION = "__TOKEN_location:string__"
SCRIPT_ID = "__SCRIPT_ID__"
TTY = "/dev/ttyAMA0"


def emit(obj: dict) -> None:
    with open(TTY, "w") as f:
        f.write(json.dumps(obj) + "\n")
        f.flush()


def is_unresolved(value: str) -> bool:
    return value.startswith("__TOKEN_")


def require_token(value: str, name: str) -> str:
    if is_unresolved(value) or not value.strip():
        raise ValueError(f"{name} token was not replaced")
    return value.strip()


def normalize_access_token(value: str) -> str:
    token = value.strip()
    if token.lower().startswith("bearer "):
        token = token[7:].strip()
    if not token:
        raise ValueError("access_token is empty")
    # Google OAuth access tokens are typically opaque (often starting with "ya29.").
    # A 3-part JWT here is usually an ID token, which cannot call Calendar APIs.
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
            "and the target calendar is writable by that account."
        )
    return payload


def clean_optional(value: str, default: Optional[str] = None) -> Optional[str]:
    if is_unresolved(value):
        return default
    cleaned = value.strip()
    if not cleaned:
        return default
    return cleaned


def create_event(
    access_token: str,
    calendar_id: str,
    summary: str,
    start_time: str,
    end_time: str,
    timezone: str,
    description: Optional[str],
    location: Optional[str],
) -> dict:
    encoded_calendar = urllib.parse.quote(calendar_id, safe="")
    url = f"https://www.googleapis.com/calendar/v3/calendars/{encoded_calendar}/events"

    body = {
        "summary": summary,
        "start": {"dateTime": start_time, "timeZone": timezone},
        "end": {"dateTime": end_time, "timeZone": timezone},
    }
    if description:
        body["description"] = description
    if location:
        body["location"] = location

    req = urllib.request.Request(
        url,
        method="POST",
        headers={
            "Authorization": f"Bearer {access_token}",
            "Content-Type": "application/json",
            "Accept": "application/json",
            "User-Agent": "smcp-google-calendar/1.0",
        },
        data=json.dumps(body).encode("utf-8"),
    )

    with urllib.request.urlopen(req, timeout=20) as resp:
        payload = json.loads(resp.read().decode())

    return {
        "calendar_id": calendar_id,
        "id": payload.get("id"),
        "status": payload.get("status"),
        "summary": payload.get("summary"),
        "start": payload.get("start", {}).get("dateTime") or payload.get("start", {}).get("date"),
        "end": payload.get("end", {}).get("dateTime") or payload.get("end", {}).get("date"),
        "html_link": payload.get("htmlLink"),
    }


if __name__ == "__main__":
    try:
        access_token = normalize_access_token(require_token(ACCESS_TOKEN, "access_token"))
        summary = require_token(SUMMARY, "summary")
        start_time = require_token(START_TIME, "start_time")
        end_time = require_token(END_TIME, "end_time")

        calendar_id = clean_optional(CALENDAR_ID, "primary")
        timezone = clean_optional(TIMEZONE, "UTC")
        description = clean_optional(DESCRIPTION)
        location = clean_optional(LOCATION)

        if calendar_id is None:
            calendar_id = "primary"
        if timezone is None:
            timezone = "UTC"

        result = create_event(
            access_token=access_token,
            calendar_id=calendar_id,
            summary=summary,
            start_time=start_time,
            end_time=end_time,
            timezone=timezone,
            description=description,
            location=location,
        )
        emit({"type": "result", "script": SCRIPT_ID, "ok": True, "output": json.dumps(result)})
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
