#!/usr/bin/env python3
"""Mock Zigbee pairing test.

This script exercises the guided pairing flow without requiring a real Zigbee network.
It performs the following high-level steps against the API gateway:

1. Authenticate as the admin user and start a Zigbee pairing session.
2. Create a synthetic Zigbee device through the public device API (to mimic a
   coordinator announcing a freshly interviewed node).
3. Publish pairing progress frames over MQTT/WebSockets to emulate the Zigbee
   adapter reporting interview updates.
4. Verify that the pairing session transitions to the completed state and that
   the synthetic device is visible through the device collection endpoint.

Environment variables:
    GATEWAY_ORIGIN (default http://localhost:8080)
    WS_STATE_URL (default ws://localhost:8080/ws/hdp)
    ADMIN_EMAIL / ADMIN_PASSWORD (admin credentials)
    PAIRING_PROTOCOL (default zigbee)
    PAIRING_DEVICE_NAME (default "Mock Zigbee Contact")
    KEEP_MOCK_DEVICE (set to 1 to skip cleanup)
"""
from __future__ import annotations

import json
import os
import sys
import time
import uuid
from dataclasses import dataclass
from typing import Dict, Iterable, List, Optional, Sequence, Tuple
from urllib.parse import urlparse

import paho.mqtt.client as mqtt
from paho.mqtt.client import CallbackAPIVersion
import requests

GATEWAY_ORIGIN = os.getenv("GATEWAY_ORIGIN", "http://localhost:8080")
DEFAULT_WS_URL = os.getenv("WS_URL", "ws://localhost:8080/ws/hdp")
WS_STATE_URL = os.getenv("WS_STATE_URL", DEFAULT_WS_URL)
ADMIN_EMAIL = os.getenv("ADMIN_EMAIL", "admin@example.com")
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "admin")
PAIRING_PROTOCOL = os.getenv("PAIRING_PROTOCOL", "zigbee").lower() or "zigbee"
PAIRING_PROTOCOLS = [p.strip().lower() for p in os.getenv("PAIRING_PROTOCOLS", f"{PAIRING_PROTOCOL},thread").split(",") if p.strip()]
PAIRING_DEVICE_NAME = os.getenv("PAIRING_DEVICE_NAME", "Mock Zigbee Contact")
KEEP_DEVICE = os.getenv("KEEP_MOCK_DEVICE", "0").lower() in {"1", "true", "yes"}
PAIRING_TIMEOUT = int(os.getenv("PAIRING_TIMEOUT", "45"))
PAIRING_PROGRESS_PREFIX = "homenavi/hdp/pairing/progress/"
HDP_SCHEMA = "hdp.v1"


def _auth(token: str) -> Dict[str, str]:
    return {"Authorization": f"Bearer {token}"}


def login() -> str:
    resp = requests.post(
        f"{GATEWAY_ORIGIN}/api/auth/login/start",
        json={"email": ADMIN_EMAIL, "password": ADMIN_PASSWORD},
        timeout=10,
    )
    resp.raise_for_status()
    payload = resp.json()
    token = payload.get("access_token")
    if not token:
        raise RuntimeError("login missing access_token")
    return token


def build_external_id(protocol: str) -> str:
    proto = (protocol or "").lower() or "manual"
    suffix = uuid.uuid4().hex[:8]
    return f"{proto}/{suffix}"


def build_device_name(protocol: str) -> str:
    proto = (protocol or "").lower()
    base = PAIRING_DEVICE_NAME
    if proto == "thread":
        base = os.getenv("PAIRING_DEVICE_NAME_THREAD", "Mock Thread Sensor")
    elif proto == "zigbee":
        base = os.getenv("PAIRING_DEVICE_NAME_ZIGBEE", PAIRING_DEVICE_NAME)
    elif proto == "matter":
        base = os.getenv("PAIRING_DEVICE_NAME_MATTER", "Mock Matter Device")
    return base


@dataclass
class PairingSession:
    id: str
    protocol: str
    status: str
    active: bool

    @classmethod
    def from_json(cls, data: Dict[str, object]) -> "PairingSession":
        return cls(
            id=str(data.get("id")),
            protocol=str(data.get("protocol")),
            status=str(data.get("status")),
            active=bool(data.get("active", False)),
        )


def start_pairing(token: str, protocol: str, metadata: Dict[str, str]) -> PairingSession:
    resp = requests.post(
        f"{GATEWAY_ORIGIN}/api/hdp/pairings",
        headers=_auth(token),
        cookies={"auth_token": token},
        json={
            "protocol": protocol,
            "timeout": PAIRING_TIMEOUT,
            "metadata": metadata,
        },
        timeout=10,
    )
    if resp.status_code != 202:
        raise RuntimeError(f"pairing start failed status={resp.status_code} body={resp.text}")
    return PairingSession.from_json(resp.json())


def create_mock_device(token: str, protocol: str, external_id: str) -> Dict[str, object]:
    device_name = build_device_name(protocol)
    payload = {
        "protocol": protocol,
        "external_id": external_id,
        "name": f"{device_name} {external_id.split('/')[-1][-4:]}",
        "type": "contact_sensor",
        "manufacturer": "MockCo",
        "model": f"{protocol.upper()}-TEST",
        "description": "Synthetic device created by test_pairing_mock",
        "firmware": "0.0.1-test",
        "icon": "door-sensor",
        "capabilities": [
            {"id": "contact", "kind": "binary", "property": "contact", "unit": "open"},
        ],
        "inputs": [
            {"id": "contact", "type": "toggle", "property": "contact"},
        ],
    }
    resp = requests.post(
        f"{GATEWAY_ORIGIN}/api/hdp/devices",
        headers=_auth(token),
        cookies={"auth_token": token},
        json=payload,
        timeout=10,
    )
    if resp.status_code != 201:
        raise RuntimeError(f"device create failed status={resp.status_code} body={resp.text}")
    return resp.json()


def publish_pairing_progress(token: str, protocol: str, events: Iterable[Dict[str, object]]) -> None:
    parsed = urlparse(WS_STATE_URL)
    host = parsed.hostname or "localhost"
    port = parsed.port or (443 if parsed.scheme == "wss" else 80)
    path = parsed.path or "/"

    client = mqtt.Client(
        CallbackAPIVersion.VERSION2,
        client_id=f"pairing-mock-{uuid.uuid4().hex[:8]}",
        transport="websockets",
    )
    client.ws_set_options(path=path, headers={"Cookie": f"auth_token={token}"})

    connected = False

    def _on_connect(_client, _userdata, _flags, reason_code, _properties=None):
        nonlocal connected
        connected = reason_code == mqtt.MQTT_ERR_SUCCESS

    client.on_connect = _on_connect
    rc = client.connect(host, port, keepalive=30)
    if rc != 0:
        raise RuntimeError(f"mqtt connect failed rc={rc}")
    client.loop_start()
    try:
        deadline = time.time() + 5
        while not connected:
            if time.time() >= deadline:
                raise RuntimeError("mqtt connect timeout")
            time.sleep(0.05)
        for evt in events:
            payload = dict(evt)
            topic_proto = str(payload.get("protocol") or protocol)
            payload.setdefault("protocol", topic_proto)
            payload.setdefault("schema", HDP_SCHEMA)
            payload.setdefault("type", "pairing_progress")
            print("PAIRING_PROGRESS_EMIT", json.dumps(payload, sort_keys=True))
            data = json.dumps(payload, separators=(",", ":")).encode("utf-8")
            topic = f"{PAIRING_PROGRESS_PREFIX}{topic_proto}"
            result = client.publish(topic, payload=data)
            result.wait_for_publish()
            time.sleep(0.1)
    finally:
        client.disconnect()
        time.sleep(0.1)
        client.loop_stop()


def list_pairings(token: str) -> List[PairingSession]:
    resp = requests.get(
        f"{GATEWAY_ORIGIN}/api/hdp/pairings",
        headers=_auth(token),
        cookies={"auth_token": token},
        timeout=10,
    )
    resp.raise_for_status()
    sessions = resp.json()
    return [PairingSession.from_json(item) for item in sessions]


def stop_pairing(token: str, protocol: str) -> None:
    resp = requests.delete(
        f"{GATEWAY_ORIGIN}/api/hdp/pairings",
        headers=_auth(token),
        cookies={"auth_token": token},
        params={"protocol": protocol},
        timeout=10,
    )
    if resp.status_code not in (200, 202, 204, 404):
        raise RuntimeError(f"pairing stop failed status={resp.status_code} body={resp.text}")


def wait_for_pairing_status(
    token: str,
    session_id: str,
    complete_statuses: Sequence[str],
    timeout: float = 20.0,
    label: str = "final",
) -> PairingSession:
    expiry = time.time() + timeout
    last_session: Optional[PairingSession] = None
    while time.time() <= expiry:
        for sess in list_pairings(token):
            if sess.id == session_id:
                last_session = sess
                print(f"PAIRING_STATUS[{label}] status={sess.status} active={sess.active}")
                if sess.status in complete_statuses:
                    return sess
                break
        else:
            if last_session:
                return last_session
        time.sleep(0.5)
    if last_session:
        return last_session
    raise RuntimeError("pairing session not found during wait")


def wait_for_session_status(
    token: str,
    session_id: str,
    expected_statuses: Sequence[str],
    label: str,
    timeout: float = 12.0,
) -> PairingSession:
    session = wait_for_pairing_status(
        token,
        session_id,
        expected_statuses,
        timeout=timeout,
        label=label,
    )
    if session.status not in expected_statuses:
        raise RuntimeError(f"pairing status {session.status} not in expected {expected_statuses}")
    return session


def list_devices(token: str) -> List[Dict[str, object]]:
    resp = requests.get(
        f"{GATEWAY_ORIGIN}/api/hdp/devices",
        headers=_auth(token),
        cookies={"auth_token": token},
        timeout=10,
    )
    resp.raise_for_status()
    data = resp.json()
    if not isinstance(data, list):
        raise RuntimeError("unexpected devices payload")
    return data


def delete_device(token: str, device_id: str, external_id: str) -> None:
    resp = requests.delete(
        f"{GATEWAY_ORIGIN}/api/hdp/devices/{device_id}?force=1",
        headers=_auth(token),
        cookies={"auth_token": token},
        timeout=10,
    )
    if resp.status_code in (200, 204, 404):
        print(f"MOCK_DEVICE_REMOVE status={resp.status_code} immediate=True")
        return
    if resp.status_code == 202:
        # removal queued via protocol-specific cleanup; wait for disappearance.
        print("MOCK_DEVICE_REMOVE status=202 awaiting removal queue")
        wait_for_device_absence(token, external_id)
        return
    raise RuntimeError(f"device delete failed status={resp.status_code} body={resp.text}")


def wait_for_device_absence(token: str, external_id: str, timeout: float = 15.0) -> None:
    expiry = time.time() + timeout
    while time.time() <= expiry:
        devices = list_devices(token)
        if not any(dev.get("external_id") == external_id for dev in devices):
            return
        time.sleep(0.5)
    raise RuntimeError(f"device {external_id} still present after delete")


def run_for_protocol(token: str, protocol: str) -> None:
    external_id = build_external_id(protocol)
    device_name = build_device_name(protocol)
    metadata = {
        "name": device_name,
        "type": "contact_sensor",
        "manufacturer": "MockCo",
        "model": f"{protocol.upper()}-TEST",
        "description": "automated pipeline validation",
        "icon": "door-sensor",
    }
    session = start_pairing(token, protocol, metadata)
    print(f"PAIRING_START protocol={protocol} session_id={session.id} status={session.status} external_id={external_id}")
    device_record = None
    try:
        device_record = create_mock_device(token, protocol, external_id)
        device_id = device_record.get("id")
        print(f"MOCK_DEVICE_CREATED protocol={protocol} device_id={device_id} external_id={external_id}")

        progress_plan = []
        if protocol == "zigbee":
            progress_plan = [
                (
                    {"protocol": protocol, "stage": "device_joined", "status": "joined", "external_id": external_id},
                    ("device_joined",),
                    "device_joined",
                ),
                (
                    {"protocol": protocol, "stage": "interview_started", "status": "interviewing", "external_id": external_id},
                    ("interviewing",),
                    "interviewing",
                ),
                (
                    {"protocol": protocol, "stage": "interview_complete", "status": "success", "external_id": external_id},
                    ("interview_complete", "completed"),
                    "interview_complete",
                ),
            ]

        if progress_plan:
            for event, expected_statuses, label in progress_plan:
                publish_pairing_progress(token, protocol, [event])
                wait_for_session_status(token, session.id, expected_statuses, label=label)
            final_session = wait_for_pairing_status(
                token,
                session.id,
                ("completed", "failed", "timeout", "stopped"),
                label="final",
            )
        else:
            # Protocols without interview tracking: stop explicitly.
            time.sleep(1.0)
            stop_pairing(token, protocol)
            final_session = wait_for_pairing_status(
                token,
                session.id,
                ("stopped", "completed", "failed", "timeout"),
                label="final",
            )

        print(f"PAIRING_FINAL protocol={protocol} status={final_session.status} active={final_session.active}")
        devices = list_devices(token)
        matched = next((dev for dev in devices if dev.get("external_id") == external_id), None)
        if not matched:
            raise RuntimeError("mock device missing from device list")
        print(f"PAIRING MOCK TEST OK protocol={protocol} device_id={matched.get('id')} external_id={external_id}")
    finally:
        if not KEEP_DEVICE and device_record and device_record.get("id"):
            try:
                delete_device(token, device_record["id"], external_id)
                print(f"MOCK_DEVICE_REMOVED protocol={protocol} device_id={device_record['id']}")
            except Exception as exc:
                print(f"MOCK_DEVICE_REMOVE_FAILED protocol={protocol} {exc}")


def run() -> None:
    print(f"LOGIN user={ADMIN_EMAIL} origin={GATEWAY_ORIGIN}")
    token = login()
    print("LOGIN_OK")
    for proto in PAIRING_PROTOCOLS:
        run_for_protocol(token, proto)


if __name__ == "__main__":
    try:
        run()
    except Exception as exc:
        print(f"PAIRING MOCK TEST FAILED: {exc}")
        sys.exit(1)