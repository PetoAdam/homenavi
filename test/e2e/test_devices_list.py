#!/usr/bin/env python3
"""Device listing test:
1. Authenticate against the API gateway.
2. Retrieve device metadata from the REST API.
3. Collect retained device state snapshots via MQTT.
4. Emit a compact summary and store a JSON snapshot for further inspection.

Environment:
    GATEWAY_ORIGIN (default http://localhost:8080)
    WS_STATE_URL (default ws://localhost:8080/ws/hdp)
    ADMIN_EMAIL / ADMIN_PASSWORD
    REQUIRE_DEVICES=1 (enforce at least one)
    VERBOSE=1 for frame logging
    SHOW_METADATA=1 to dump raw capability/input details per device
    DEVICES_SNAPSHOT_PATH (default devices_snapshot.json)
"""

from __future__ import annotations

import json
import os
import sys
import time
from typing import Any, Callable, Dict, Iterable, List, Tuple
from urllib.parse import urlparse

import paho.mqtt.client as mqtt
import requests

GATEWAY_ORIGIN = os.getenv("GATEWAY_ORIGIN", "http://localhost:8080")
DEFAULT_WS_URL = os.getenv("WS_URL", "ws://localhost:8080/ws/hdp")
WS_STATE_URL = os.getenv("WS_STATE_URL", DEFAULT_WS_URL)
ADMIN_EMAIL = os.getenv("ADMIN_EMAIL", "admin@example.com")
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "admin")
VERBOSE = os.getenv("VERBOSE", "0").lower() in ("1", "true", "yes")
SHOW_METADATA = os.getenv("SHOW_METADATA", "0").lower() in ("1", "true", "yes")
REQUIRE_DEVICES = os.getenv("REQUIRE_DEVICES", "0").lower() in ("1", "true", "yes")
SNAPSHOT_PATH = os.getenv("DEVICES_SNAPSHOT_PATH", "devices_snapshot.json")
HDP_PREFIX = "homenavi/hdp/"
STATE_PREFIX = f"{HDP_PREFIX}device/state/"

CollectHandler = Callable[[str, bytes], None]
def login() -> str:
    response = requests.post(
        f"{GATEWAY_ORIGIN}/api/auth/login/start",
        json={"email": ADMIN_EMAIL, "password": ADMIN_PASSWORD},
        timeout=5,
    )
    response.raise_for_status()
    data = response.json()
    token = data.get("access_token")
    if not token:
        raise RuntimeError("missing access_token in login response")
    return token

def mqtt_collect(
    ws_url: str,
    token: str,
    subscriptions: Iterable[Tuple[str, int]],
    handler: CollectHandler,
    label: str,
    max_wait: float = 6.0,
    idle_timeout: float = 0.7,
) -> int:
    """Subscribe to topics via MQTT over WebSocket and invoke handler for each message."""

    parsed = urlparse(ws_url)
    host = parsed.hostname or "localhost"
    if parsed.scheme == "wss":
        default_port = 443
    elif parsed.scheme == "ws":
        default_port = 80
    else:
        raise ValueError(f"Unsupported WebSocket scheme in {ws_url}")
    port = parsed.port or default_port
    path = parsed.path or "/"

    client = mqtt.Client(
        client_id=f"{label}-{int(time.time() * 1000)}",
        transport="websockets",
    )
    client.ws_set_options(path=path, headers={"Cookie": f"auth_token={token}"})

    message_count = 0
    last_msg_at = 0.0
    connect_rc = {"value": None}

    subs = list(subscriptions)
    if not subs:
        raise ValueError("No subscriptions provided")

    def _on_connect(c: mqtt.Client, _userdata: Any, _flags: Dict[str, Any], rc: int) -> None:
        connect_rc["value"] = rc
        if rc != 0:
            return
        c.subscribe(subs)

    def _on_message(_c: mqtt.Client, _userdata: Any, msg: mqtt.MQTTMessage) -> None:
        nonlocal message_count, last_msg_at
        message_count += 1
        last_msg_at = time.time()
        if VERBOSE:
            print(f"MSG[{label}] {msg.topic} {len(msg.payload)}B")
        handler(msg.topic, msg.payload)

    client.on_connect = _on_connect
    client.on_message = _on_message

    rc = client.connect(host, port, keepalive=30)
    if rc != 0:
        raise RuntimeError(f"{label} connect failed rc={rc}")

    client.loop_start()
    start = time.time()
    try:
        while True:
            now = time.time()
            if connect_rc["value"] not in (None, 0):
                raise RuntimeError(f"{label} connect rc={connect_rc['value']}")
            if now - start >= max_wait:
                break
            if message_count > 0 and last_msg_at > 0 and (now - last_msg_at) >= idle_timeout:
                break
            time.sleep(0.05)
    finally:
        try:
            client.disconnect()
            time.sleep(0.1)
        except Exception:
            pass
        client.loop_stop()

    return message_count

def safe_json(payload: bytes) -> Any:
    if isinstance(payload, (bytes, bytearray)):
        text = payload.decode("utf-8", errors="ignore").strip()
    else:
        text = str(payload).strip()
    if not text:
        return None
    try:
        return json.loads(text)
    except Exception:
        return text

def _ensure_json(obj: Dict[str, Any], key: str) -> None:
    val = obj.get(key)
    if isinstance(val, (list, dict)) or val is None:
        return
    if isinstance(val, (bytes, bytearray)):
        try:
            val = val.decode("utf-8")
        except Exception:
            val = None
    if isinstance(val, str):
        txt = val.strip()
        if not txt:
            obj[key] = [] if key in ("capabilities", "inputs") else None
            return
        try:
            obj[key] = json.loads(txt)
        except Exception:
            # Leave original value if JSON parsing fails.
            return

def normalize_device_entry(entry: Dict[str, Any]) -> Dict[str, Any]:
    if not isinstance(entry, dict):
        return entry
    for field in ("capabilities", "inputs"):
        _ensure_json(entry, field)
        if entry.get(field) is None:
            entry[field] = []
    desc = entry.get("description")
    if isinstance(desc, (bytes, bytearray)):
        entry["description"] = desc.decode("utf-8", errors="ignore")
    return entry

def _format_unit(value: Any) -> str:
    if value is None:
        return ""
    try:
        text = str(value).strip()
    except Exception:
        return ""
    return text


def summarize_capabilities(caps: Any, limit: int = 4) -> str:
    if isinstance(caps, (bytes, bytearray)):
        try:
            caps = caps.decode("utf-8")
        except Exception:
            caps = None
    if isinstance(caps, str):
        try:
            caps = json.loads(caps)
        except Exception:
            return "caps=?"
    if isinstance(caps, dict):
        caps = [caps]
    if not isinstance(caps, list) or len(caps) == 0:
        return "caps=0"
    labels: List[str] = []
    for cap in caps[:limit]:
        if isinstance(cap, dict):
            ident = cap.get("id") or cap.get("property") or cap.get("name")
            kind = cap.get("kind") or cap.get("type")
            unit = _format_unit(cap.get("unit") or cap.get("units"))
            unit_suffix = f"({unit})" if unit else ""
            if ident and kind:
                labels.append(f"{ident}:{kind}{unit_suffix}")
            elif ident:
                labels.append(f"{ident}{unit_suffix}")
            elif kind:
                labels.append(f"{kind}{unit_suffix}")
    extra = f" (+{len(caps) - limit})" if len(caps) > limit else ""
    return f"caps={len(caps)}[{', '.join(labels)}{extra}]"

def summarize_inputs(inputs: Any, limit: int = 3) -> str:
    if isinstance(inputs, (bytes, bytearray)):
        try:
            inputs = inputs.decode("utf-8")
        except Exception:
            inputs = None
    if isinstance(inputs, str):
        try:
            inputs = json.loads(inputs)
        except Exception:
            return "inputs=?"
    if isinstance(inputs, dict):
        inputs = [inputs]
    if not isinstance(inputs, list) or len(inputs) == 0:
        return "inputs=0"
    labels: List[str] = []
    for inp in inputs[:limit]:
        if isinstance(inp, dict):
            ident = inp.get("id")
            typ = inp.get("type")
            unit = _format_unit(inp.get("unit") or inp.get("units"))
            unit_suffix = f"({unit})" if unit else ""
            if ident and typ:
                labels.append(f"{ident}:{typ}{unit_suffix}")
            elif typ:
                labels.append(f"{typ}{unit_suffix}")
            elif ident:
                labels.append(f"{ident}{unit_suffix}")
    extra = f" (+{len(inputs) - limit})" if len(inputs) > limit else ""
    return f"inputs={len(inputs)}[{', '.join(labels)}{extra}]"

def fetch_device_metadata(token: str) -> Tuple[Dict[str, Dict[str, Any]], Dict[str, str]]:
    url = f"{GATEWAY_ORIGIN}/api/hdp/devices"
    headers = {"Authorization": f"Bearer {token}"}
    cookies = {"auth_token": token}
    response = requests.get(url, headers=headers, cookies=cookies, timeout=10)
    response.raise_for_status()
    payload = response.json()
    if not isinstance(payload, list):
        raise RuntimeError("unexpected metadata payload shape")

    devices: Dict[str, Dict[str, Any]] = {}
    id_to_external: Dict[str, str] = {}
    for item in payload:
        if not isinstance(item, dict):
            continue
        entry = normalize_device_entry(dict(item))
        dev_id = entry.get("id")
        if not isinstance(dev_id, str) or not dev_id:
            continue
        devices[dev_id] = entry
        external = entry.get("external_id")
        if isinstance(external, str) and external:
            id_to_external[dev_id] = external
        else:
            id_to_external[dev_id] = dev_id
    return devices, id_to_external

def fetch_device_states(
    token: str,
    devices: Dict[str, Dict[str, Any]],
    id_to_external: Dict[str, str],
) -> Dict[str, Any]:
    states: Dict[str, Any] = {}

    def handler(topic: str, payload: bytes) -> None:
        if topic.startswith(STATE_PREFIX):
            dev_id = topic[len(STATE_PREFIX) :]
            data = safe_json(payload)
            states[dev_id] = data
            entry = devices.setdefault(dev_id, {"id": dev_id})
            normalize_device_entry(entry)
            entry["_last_state"] = data
            if dev_id not in id_to_external:
                ext = entry.get("external_id")
                if isinstance(ext, str) and ext:
                    id_to_external[dev_id] = ext
                else:
                    id_to_external[dev_id] = dev_id

    mqtt_collect(
        WS_STATE_URL,
        token,
        [(STATE_PREFIX + "#", 0)],
        handler,
        "device-state",
    )
    return states

def run() -> None:
    token = login()
    devices, id_to_external = fetch_device_metadata(token)
    metadata_keys = set(devices.keys())
    states = fetch_device_states(token, devices, id_to_external)

    # Merge state payloads for any devices not seen via metadata.
    for dev_id, state in states.items():
        entry = devices.setdefault(dev_id, {"id": dev_id})
        normalize_device_entry(entry)
        entry["_last_state"] = state
        if dev_id not in id_to_external:
            external = entry.get("external_id")
            if isinstance(external, str) and external:
                id_to_external[dev_id] = external
            else:
                id_to_external[dev_id] = dev_id

    logical: Dict[str, Dict[str, Any]] = {}
    ignored = 0
    for dev_id, data in devices.items():
        if not isinstance(data, dict):
            ignored += 1
            continue
        include = dev_id in metadata_keys
        if not include:
            for field in ("capabilities", "inputs", "manufacturer", "model", "protocol", "definition"):
                value = data.get(field)
                if isinstance(value, (list, dict)) and value:
                    include = True
                    break
                if isinstance(value, str) and value.strip():
                    include = True
                    break
        if include:
            logical[dev_id] = normalize_device_entry(data)
        else:
            ignored += 1

    count = len(logical)
    if ignored:
        print(f"IGNORED_STATE_ONLY {ignored}")

    states_attached = True
    for entry in logical.values():
        dev_id = entry.get("id")
        if dev_id and "_last_state" not in entry:
            states_attached = False
            break

    print(f"DEVICES_COUNT {count}")
    sorted_keys = sorted(logical.keys())
    records: List[Dict[str, Any]] = []
    for dev_id in sorted_keys:
        entry = logical[dev_id]
        display_key = id_to_external.get(dev_id, entry.get("external_id") or dev_id)
        record = dict(entry)
        state_obj = record.pop("_last_state", None)
        caps_summary = summarize_capabilities(entry.get("capabilities"))
        inputs_summary = summarize_inputs(entry.get("inputs"))
        manufacturer = entry.get("manufacturer") or "-"
        model = entry.get("model") or "-"
        firmware = entry.get("firmware") or entry.get("software_build_id") or "-"
        description_val = entry.get("description")
        if isinstance(description_val, str):
            normalized_desc = " ".join(description_val.split())
        else:
            normalized_desc = ""
        if normalized_desc:
            short_desc = normalized_desc if len(normalized_desc) <= 60 else normalized_desc[:57] + "..."
        else:
            short_desc = "-"
        dev_id_label = entry.get("id") or "-"
        dev_type = entry.get("type") or entry.get("definition", {}).get("type") or "-"
        units_by_property: Dict[str, str] = {}
        caps_list = entry.get("capabilities")
        if isinstance(caps_list, list):
            for cap in caps_list:
                if isinstance(cap, dict):
                    unit = _format_unit(cap.get("unit") or cap.get("units"))
                    if not unit:
                        continue
                    for key in (cap.get("property"), cap.get("id"), cap.get("name")):
                        if key:
                            units_by_property[str(key)] = unit
        state_keys: List[str] = []
        if isinstance(state_obj, dict):
            for key in list(state_obj.keys())[:5]:
                label = str(key)
                unit = units_by_property.get(label)
                if unit:
                    state_keys.append(f"{label}({unit})")
                else:
                    state_keys.append(label)
        print(
            f"- {display_key} id={dev_id_label} type={dev_type} manufacturer={manufacturer} "
            f"model={model} description={short_desc} firmware={firmware} {caps_summary} {inputs_summary} state_keys={state_keys}"
        )
        if SHOW_METADATA:
            print(json.dumps(entry, indent=2)[:2000])
            if isinstance(state_obj, dict):
                print("STATE_SAMPLE " + json.dumps({k: state_obj[k] for k in state_keys}, indent=2))
        if state_obj is not None:
            record["state"] = state_obj
        records.append(record)

    print(f"ALL_DEVICE_STATES_ATTACHED {1 if states_attached else 0}")

    try:
        with open(SNAPSHOT_PATH, "w", encoding="utf-8") as fh:
            json.dump(records, fh, indent=2, default=str)
        print(f"DEVICE_DATA_SNAPSHOT {SNAPSHOT_PATH} records={len(records)}")
    except Exception as exc:
        print(f"DEVICE_DATA_SNAPSHOT_FAILED {exc}")

    if count == 0:
        print("No devices discovered via retained topics.")
    if REQUIRE_DEVICES and count == 0:
        raise SystemExit(1)

if __name__ == "__main__":
    try:
        run()
        print("DEVICE LIST TEST OK")
    except Exception as exc:
        print("DEVICE LIST TEST FAILED", exc)
        sys.exit(1)
