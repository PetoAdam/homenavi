#!/usr/bin/env python3
"""History service E2E query script.

Flow:
1) Login via API gateway: POST /api/auth/login/start
2) Fetch actual devices from device-hub: GET /api/hdp/devices
3) Query history-service endpoints via API gateway:
   - GET /api/history/health
   - GET /api/history/state for each real device_id

Environment:
  GATEWAY_ORIGIN    (default http://localhost:8080)
  ADMIN_EMAIL       (default admin@example.com)
  ADMIN_PASSWORD    (default admin)
  HISTORY_LIMIT     (default 5)
  HISTORY_ORDER     (default desc)
  HISTORY_MAX_DEVICES (default 20)
"""

from __future__ import annotations

import json
import os
import sys
from typing import Any, Dict, List, Optional

import requests


GATEWAY_ORIGIN = os.getenv("GATEWAY_ORIGIN", "http://localhost:8080").rstrip("/")
ADMIN_EMAIL = os.getenv("ADMIN_EMAIL", "admin@example.com")
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "admin")
HISTORY_LIMIT = int(os.getenv("HISTORY_LIMIT", "5"))
HISTORY_ORDER = os.getenv("HISTORY_ORDER", "desc")
HISTORY_MAX_DEVICES = int(os.getenv("HISTORY_MAX_DEVICES", "20"))


def _print_json(label: str, obj: Any) -> None:
    print(f"\n== {label} ==")
    print(json.dumps(obj, indent=2, sort_keys=True, default=str))


def _print_response(label: str, r: requests.Response) -> None:
    print(f"\n== {label} ==")
    print(f"{r.request.method} {r.request.url}")
    print(f"Status: {r.status_code}")
    ct = r.headers.get("content-type", "")
    if "application/json" in ct:
        try:
            _print_json("body", r.json())
        except Exception:
            print(r.text)
    else:
        print(r.text)


def login_start(session: requests.Session) -> str:
    url = f"{GATEWAY_ORIGIN}/api/auth/login/start"
    r = session.post(url, json={"email": ADMIN_EMAIL, "password": ADMIN_PASSWORD}, timeout=10)
    r.raise_for_status()
    data: Dict[str, Any] = r.json()

    token = data.get("access_token")
    if not token:
        if data.get("2fa_required"):
            raise RuntimeError(
                "Login requires 2FA for this user. "
                "Use a non-2FA user or complete /api/auth/login/finish first."
            )
        raise RuntimeError("missing access_token in /api/auth/login/start response")
    return str(token)


def get_json(
    session: requests.Session,
    path: str,
    *,
    token: Optional[str] = None,
    params: Optional[dict] = None,
) -> requests.Response:
    url = f"{GATEWAY_ORIGIN}{path}"
    headers: Dict[str, str] = {}
    cookies: Dict[str, str] = {}
    if token:
        headers["Authorization"] = f"Bearer {token}"
        # Some gateway middleware paths also accept the token in the cookie.
        cookies["auth_token"] = token
    return session.get(url, headers=headers, cookies=cookies, params=params, timeout=15)


def fetch_device_ids(session: requests.Session, token: str) -> List[str]:
    r = get_json(session, "/api/hdp/devices", token=token)
    r.raise_for_status()
    payload = r.json()
    if not isinstance(payload, list):
        raise RuntimeError("unexpected /api/hdp/devices response shape (expected list)")

    ids: List[str] = []
    for item in payload:
        if not isinstance(item, dict):
            continue
        # Prefer the canonical HDP id (e.g. "zigbee/0x...") which matches MQTT topics.
        hdp_id = item.get("device_id")
        if isinstance(hdp_id, str) and hdp_id:
            ids.append(hdp_id)
            continue
        # Fallbacks (older payload shapes)
        dev_id = item.get("id")
        if isinstance(dev_id, str) and dev_id:
            ids.append(dev_id)

    # de-dupe, preserve order
    seen = set()
    ordered: List[str] = []
    for dev_id in ids:
        if dev_id in seen:
            continue
        seen.add(dev_id)
        ordered.append(dev_id)
    return ordered


def main() -> int:
    s = requests.Session()

    token = login_start(s)
    print(f"Logged in as {ADMIN_EMAIL} (token acquired)")

    # History health
    r = get_json(s, "/api/history/health", token=token)
    _print_response("history health", r)
    r.raise_for_status()

    device_ids = fetch_device_ids(s, token)
    if not device_ids:
        print("No devices returned from /api/hdp/devices; nothing to query.")
        return 0

    if HISTORY_MAX_DEVICES > 0:
        device_ids = device_ids[:HISTORY_MAX_DEVICES]

    print(f"Querying history for {len(device_ids)} device(s)")

    any_fail = False
    any_points = False
    for dev_id in device_ids:
        r = get_json(
            s,
            "/api/history/state",
            token=token,
            params={
                "device_id": dev_id,
                "limit": HISTORY_LIMIT,
                "order": HISTORY_ORDER,
            },
        )
        _print_response(f"history state device_id={dev_id}", r)
        if r.status_code >= 400:
            any_fail = True
        else:
            try:
                body = r.json()
                pts = body.get("points") if isinstance(body, dict) else None
                if isinstance(pts, list) and len(pts) > 0:
                    any_points = True
            except Exception:
                pass

    if not any_fail and not any_points:
        print(
            "\nNOTE: history returned 0 points for all devices. "
            "If you expect immediate data, set HISTORY_INGEST_RETAINED=true and restart history-service."
        )

    return 2 if any_fail else 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except requests.HTTPError as e:
        resp = getattr(e, "response", None)
        if resp is not None:
            _print_response("error", resp)
        print(f"HTTP error: {e}", file=sys.stderr)
        raise SystemExit(2)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        raise SystemExit(2)
