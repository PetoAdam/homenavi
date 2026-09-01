"""Black-box MQTT-to-history smoke test for a deployed Homenavi stack."""

from __future__ import annotations

import json
import os
import time
import uuid

import paho.mqtt.client as mqtt
import requests


GATEWAY_URL = os.getenv("HOMENAVI_GATEWAY_URL", "http://127.0.0.1:8080").rstrip("/")
MQTT_HOST = os.getenv("HOMENAVI_MQTT_HOST", "127.0.0.1")
MQTT_PORT = int(os.getenv("HOMENAVI_MQTT_PORT", "1883"))
HISTORY_TIMEOUT_SECONDS = float(os.getenv("HOMENAVI_HISTORY_TIMEOUT_SECONDS", "30"))


def login() -> str:
    response = requests.post(
        f"{GATEWAY_URL}/api/auth/login/start",
        json={"email": "admin@example.com", "password": "admin"},
        timeout=10,
    )
    response.raise_for_status()
    token = response.json().get("access_token")
    assert token, "login response did not include access_token"
    return str(token)


def publish_state(device_id: str, payload: dict[str, object]) -> None:
    client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=f"homenavi-smoke-{uuid.uuid4().hex}")
    connected = False

    def on_connect(_client, _userdata, _connect_flags, reason_code, _properties=None):
        nonlocal connected
        connected = reason_code == 0

    client.on_connect = on_connect
    client.connect(MQTT_HOST, MQTT_PORT, keepalive=20)
    client.loop_start()
    try:
        deadline = time.monotonic() + 10
        while not connected:
            if time.monotonic() >= deadline:
                raise AssertionError("timed out connecting to MQTT broker")
            time.sleep(0.05)
        result = client.publish(
            f"homenavi/hdp/device/state/{device_id}",
            json.dumps(payload, separators=(",", ":")),
            qos=1,
            retain=False,
        )
        result.wait_for_publish(timeout=10)
        assert result.is_published(), "MQTT state event was not published"
    finally:
        client.disconnect()
        client.loop_stop()


def test_mqtt_state_is_persisted_and_queryable() -> None:
    token = login()
    device_id = f"smoke/{uuid.uuid4().hex}"
    payload = {"source": "compose-smoke", "value": uuid.uuid4().hex}
    publish_state(device_id, payload)

    headers = {"Authorization": f"Bearer {token}"}
    deadline = time.monotonic() + HISTORY_TIMEOUT_SECONDS
    last_response = None
    while time.monotonic() < deadline:
        response = requests.get(
            f"{GATEWAY_URL}/api/history/state",
            params={"device_id": device_id, "limit": 20, "order": "desc"},
            headers=headers,
            timeout=10,
        )
        last_response = response
        if response.status_code == 200:
            points = response.json().get("points", [])
            if any(point.get("payload") == payload for point in points):
                return
        time.sleep(0.5)

    detail = last_response.text if last_response is not None else "no history response"
    raise AssertionError(f"MQTT state was not persisted for {device_id}: {detail}")