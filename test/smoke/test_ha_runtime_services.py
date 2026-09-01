from __future__ import annotations

import time
from typing import Any

import requests


PAIRING_PROTOCOL = "mock"


def api_get(session: requests.Session, gateway_url: str, path: str, headers: dict[str, str], **kwargs: Any) -> requests.Response:
    return session.get(f"{gateway_url}{path}", headers=headers, timeout=10, **kwargs)


def api_post(session: requests.Session, gateway_url: str, path: str, headers: dict[str, str], **kwargs: Any) -> requests.Response:
    return session.post(f"{gateway_url}{path}", headers=headers, timeout=10, **kwargs)


def api_put(session: requests.Session, gateway_url: str, path: str, headers: dict[str, str], **kwargs: Any) -> requests.Response:
    return session.put(f"{gateway_url}{path}", headers=headers, timeout=10, **kwargs)


def api_patch(session: requests.Session, gateway_url: str, path: str, headers: dict[str, str], **kwargs: Any) -> requests.Response:
    return session.patch(f"{gateway_url}{path}", headers=headers, timeout=10, **kwargs)


def api_delete(session: requests.Session, gateway_url: str, path: str, headers: dict[str, str], **kwargs: Any) -> requests.Response:
    return session.delete(f"{gateway_url}{path}", headers=headers, timeout=10, **kwargs)


def wait_until(predicate, *, timeout: float = 12.0, interval: float = 0.2, message: str = "condition not met"):
    deadline = time.monotonic() + timeout
    last_value = None
    while time.monotonic() < deadline:
        last_value = predicate()
        if last_value:
            return last_value
        time.sleep(interval)
    raise AssertionError(message if last_value is None else f"{message}: {last_value}")


def list_pairings(session: requests.Session, gateway_url: str, headers: dict[str, str]) -> list[dict[str, Any]]:
    response = api_get(session, gateway_url, "/api/hdp/pairings", headers)
    response.raise_for_status()
    payload = response.json()
    assert isinstance(payload, list), f"unexpected pairing payload: {payload!r}"
    return payload


def find_pairing(pairings: list[dict[str, Any]], session_id: str) -> dict[str, Any] | None:
    for item in pairings:
        if item.get("id") == session_id:
            return item
    return None


def wait_for_pairing_status(
    session: requests.Session,
    gateway_url: str,
    headers: dict[str, str],
    session_id: str,
    statuses: set[str],
    *,
    timeout: float = 12.0,
) -> dict[str, Any]:
    last_seen: dict[str, Any] | None = None

    def _poll():
        nonlocal last_seen
        current = find_pairing(list_pairings(session, gateway_url, headers), session_id)
        if current:
            last_seen = current
            if current.get("status") in statuses:
                return current
        return None

    try:
        return wait_until(_poll, timeout=timeout, message=f"pairing {session_id} did not reach {sorted(statuses)}")
    except AssertionError:
        if last_seen and last_seen.get("status") in statuses:
            return last_seen
        raise


def wait_for_pairing_absence(
    session: requests.Session,
    gateway_url: str,
    headers: dict[str, str],
    protocol: str,
    *,
    timeout: float = 12.0,
) -> None:
    wait_until(
        lambda: not any(item.get("protocol") == protocol and item.get("active") for item in list_pairings(session, gateway_url, headers)),
        timeout=timeout,
        message=f"pairing {protocol} remained active",
    )


def wait_for_pairing_terminal_or_absence(
    session: requests.Session,
    gateway_url: str,
    headers: dict[str, str],
    session_id: str,
    statuses: set[str],
    *,
    timeout: float = 12.0,
) -> dict[str, Any] | None:
    def _poll() -> dict[str, Any] | None | bool:
        current = find_pairing(list_pairings(session, gateway_url, headers), session_id)
        if current is None:
            return None
        if current.get("status") in statuses:
            return current
        return False

    return wait_until(
        _poll,
        timeout=timeout,
        message=f"pairing {session_id} did not reach {sorted(statuses)} or disappear",
    )


def stop_pairing(session: requests.Session, gateway_url: str, headers: dict[str, str], protocol: str) -> None:
    response = api_delete(session, gateway_url, "/api/hdp/pairings", headers, params={"protocol": protocol})
    assert response.status_code in (200, 202, 204, 404), response.text


def start_pairing(
    session: requests.Session,
    gateway_url: str,
    headers: dict[str, str],
    *,
    mode: str,
    timeout_seconds: int,
    inputs: dict[str, Any] | None = None,
) -> dict[str, Any]:
    response = api_post(
        session,
        gateway_url,
        "/api/hdp/pairings",
        headers,
        json={
            "protocol": PAIRING_PROTOCOL,
            "mode": mode,
            "timeout": timeout_seconds,
            "inputs": inputs or {},
            "metadata": {
                "type": "test_device",
                "manufacturer": "HA Smoke",
                "model": "pairing-suite",
                "description": f"{mode} pairing flow",
                "icon": "test-tube",
            },
        },
    )
    assert response.status_code == 202, response.text
    payload = response.json()
    assert payload.get("id"), payload
    return payload


def ensure_pairing_stopped(session: requests.Session, gateway_url: str, headers: dict[str, str]) -> None:
    stop_pairing(session, gateway_url, headers, PAIRING_PROTOCOL)
    wait_for_pairing_absence(session, gateway_url, headers, PAIRING_PROTOCOL)


def workflow_definition() -> dict[str, Any]:
    return {
        "version": "automation",
        "nodes": [
            {"id": "trigger-1", "kind": "trigger.manual", "x": 0, "y": 0, "data": {}},
            {"id": "sleep-1", "kind": "logic.sleep", "x": 160, "y": 0, "data": {"duration_sec": 0.1}},
        ],
        "edges": [{"from": "trigger-1", "to": "sleep-1"}],
    }


def wait_for_run_success(
    session: requests.Session,
    gateway_url: str,
    headers: dict[str, str],
    run_id: str,
    *,
    timeout: float = 12.0,
) -> dict[str, Any]:
    def _poll():
        response = api_get(session, gateway_url, f"/api/automation/runs/{run_id}", headers)
        if response.status_code != 200:
            return None
        payload = response.json()
        run = payload.get("run") if isinstance(payload, dict) else None
        if isinstance(run, dict) and run.get("status") == "success":
            return payload
        return None

    return wait_until(_poll, timeout=timeout, message=f"automation run {run_id} did not finish successfully")


def test_ha_public_health_routes(session: requests.Session, gateway_url: str, auth_headers: dict[str, str]) -> None:
    paths = [
        "/health",
        "/api/gateway/health",
        "/api/auth/health",
        "/api/users/health",
        "/api/hdp/health",
        "/api/ers/health",
        "/api/history/health",
        "/api/automation/health",
    ]
    for path in paths:
        response = api_get(session, gateway_url, path, auth_headers if path.startswith("/api/") else {})
        assert response.status_code == 200, f"{path}: {response.status_code} {response.text}"


def test_device_hub_exposes_mock_pairing_config_and_integration(session: requests.Session, gateway_url: str, auth_headers: dict[str, str]) -> None:
    config_response = api_get(session, gateway_url, "/api/hdp/pairing-config", auth_headers)
    assert config_response.status_code == 200, config_response.text
    configs = config_response.json()
    assert isinstance(configs, list), configs
    mock_config = next((item for item in configs if item.get("protocol") == PAIRING_PROTOCOL), None)
    assert mock_config is not None, configs
    assert mock_config.get("supported") is True
    assert "qr_code" in (mock_config.get("flow") or {}).get("entry_modes", [])

    integrations_response = api_get(session, gateway_url, "/api/hdp/integrations", auth_headers)
    assert integrations_response.status_code == 200, integrations_response.text
    integrations = integrations_response.json()
    assert isinstance(integrations, list), integrations
    mock_integration = next((item for item in integrations if item.get("protocol") == PAIRING_PROTOCOL), None)
    assert mock_integration is not None, integrations
    assert str(mock_integration.get("status", "")).lower() == "active"


def test_pairing_qr_code_requires_input_and_can_be_cancelled(session: requests.Session, gateway_url: str, auth_headers: dict[str, str]) -> None:
    ensure_pairing_stopped(session, gateway_url, auth_headers)
    pairing = start_pairing(session, gateway_url, auth_headers, mode="qr_code", timeout_seconds=20, inputs={})
    try:
        active = wait_for_pairing_status(session, gateway_url, auth_headers, pairing["id"], {"needs_input"})
        assert active.get("active") is True
        assert "onboarding_payload" in (active.get("required_inputs") or [])

        duplicate = api_post(
            session,
            gateway_url,
            "/api/hdp/pairings",
            auth_headers,
            json={"protocol": PAIRING_PROTOCOL, "mode": "qr_code", "timeout": 20, "inputs": {}},
        )
        assert duplicate.status_code == 202, duplicate.text
        duplicate_payload = duplicate.json()
        assert duplicate_payload.get("id") == pairing["id"], duplicate_payload

        stop_pairing(session, gateway_url, auth_headers, PAIRING_PROTOCOL)
        wait_for_pairing_absence(session, gateway_url, auth_headers, PAIRING_PROTOCOL)
    finally:
        ensure_pairing_stopped(session, gateway_url, auth_headers)


def test_pairing_qr_code_times_out_and_releases_protocol(session: requests.Session, gateway_url: str, auth_headers: dict[str, str]) -> None:
    ensure_pairing_stopped(session, gateway_url, auth_headers)
    pairing = start_pairing(session, gateway_url, auth_headers, mode="qr_code", timeout_seconds=1, inputs={})
    timed_out = wait_for_pairing_terminal_or_absence(session, gateway_url, auth_headers, pairing["id"], {"timeout"}, timeout=8.0)
    if timed_out is not None:
        assert timed_out.get("status") == "timeout"
    wait_for_pairing_absence(session, gateway_url, auth_headers, PAIRING_PROTOCOL)

    restarted = start_pairing(
        session,
        gateway_url,
        auth_headers,
        mode="qr_code",
        timeout_seconds=20,
        inputs={"onboarding_payload": "MT:HA-SMOKE-RESTART"},
    )
    assert restarted.get("id") != pairing.get("id"), restarted
    try:
        assert restarted.get("protocol") == PAIRING_PROTOCOL
    finally:
        ensure_pairing_stopped(session, gateway_url, auth_headers)


def test_entity_registry_crud_and_selectors(session: requests.Session, gateway_url: str, auth_headers: dict[str, str], smoke_prefix: str) -> None:
    room_id = None
    tag_id = None
    device_id = None
    group_id = None
    room_name = f"{smoke_prefix} room"
    tag_name = f"{smoke_prefix} tag"
    group_name = f"{smoke_prefix} group"
    updated_group_name = f"{smoke_prefix} group updated"
    hdp_external = f"mock/{smoke_prefix}-device"

    try:
        response = api_post(session, gateway_url, "/api/ers/rooms/", auth_headers, json={"name": room_name})
        assert response.status_code == 201, response.text
        room_id = response.json()["id"]

        response = api_post(session, gateway_url, "/api/ers/tags/", auth_headers, json={"name": tag_name})
        assert response.status_code == 201, response.text
        tag_id = response.json()["id"]

        response = api_post(session, gateway_url, "/api/ers/devices/", auth_headers, json={"name": f"{smoke_prefix} device", "room_id": room_id})
        assert response.status_code == 201, response.text
        device_id = response.json()["id"]

        response = api_put(
            session,
            gateway_url,
            f"/api/ers/devices/{device_id}/bindings/hdp",
            auth_headers,
            json={"hdp_external_ids": [hdp_external]},
        )
        assert response.status_code == 200, response.text

        response = api_put(
            session,
            gateway_url,
            f"/api/ers/devices/{device_id}/tags",
            auth_headers,
            json={"tag_ids": [tag_id]},
        )
        assert response.status_code == 200, response.text

        response = api_post(
            session,
            gateway_url,
            "/api/ers/groups/",
            auth_headers,
            json={"name": group_name, "device_ids": [device_id]},
        )
        assert response.status_code == 201, response.text
        group = response.json()
        group_id = group["id"]
        assert len(group.get("devices") or []) == 1

        response = api_patch(session, gateway_url, f"/api/ers/groups/{group_id}", auth_headers, json={"name": updated_group_name})
        assert response.status_code == 200, response.text
        updated_group = response.json()
        assert updated_group.get("slug") == f"{smoke_prefix}-group-updated"

        for selector in (f"room:{updated_group_name}".replace(" group updated", " room"), f"tag:{tag_name}", f"group:{updated_group_name}"):
            normalized = selector.lower().replace(" ", "-")
            if selector.startswith("room:"):
                normalized = f"room:{room_name.lower().replace(' ', '-') }"
            elif selector.startswith("tag:"):
                normalized = f"tag:{tag_name.lower().replace(' ', '-') }"
            elif selector.startswith("group:"):
                normalized = f"group:{updated_group_name.lower().replace(' ', '-') }"
            response = api_post(session, gateway_url, "/api/ers/selectors/resolve", auth_headers, json={"selector": normalized})
            assert response.status_code == 200, response.text
            payload = response.json()
            assert hdp_external in (payload.get("hdp_external_ids") or []), payload
    finally:
        if group_id:
            response = api_delete(session, gateway_url, f"/api/ers/groups/{group_id}", auth_headers)
            assert response.status_code in (200, 204, 404), response.text
        if device_id:
            response = api_delete(session, gateway_url, f"/api/ers/devices/{device_id}", auth_headers)
            assert response.status_code in (200, 204, 404), response.text
        if tag_id:
            response = api_delete(session, gateway_url, f"/api/ers/tags/{tag_id}", auth_headers)
            assert response.status_code in (200, 204, 404), response.text
        if room_id:
            response = api_delete(session, gateway_url, f"/api/ers/rooms/{room_id}", auth_headers)
            assert response.status_code in (200, 204, 404), response.text


def test_automation_manual_workflow_run_succeeds(session: requests.Session, gateway_url: str, auth_headers: dict[str, str], smoke_prefix: str) -> None:
    response = api_post(
        session,
        gateway_url,
        "/api/automation/workflows",
        auth_headers,
        json={"name": f"{smoke_prefix} manual workflow", "definition": workflow_definition()},
    )
    assert response.status_code == 201, response.text
    workflow = response.json()
    workflow_id = workflow["id"]

    try:
        response = api_post(session, gateway_url, f"/api/automation/workflows/{workflow_id}/enable", auth_headers, json={})
        assert response.status_code == 200, response.text

        response = api_post(session, gateway_url, f"/api/automation/workflows/{workflow_id}/run", auth_headers, json={})
        assert response.status_code == 200, response.text
        run_id = response.json().get("run_id")
        assert run_id, response.json()

        payload = wait_for_run_success(session, gateway_url, auth_headers, run_id)
        run = payload["run"]
        steps = payload["steps"]
        assert run.get("workflow_id") == workflow_id
        assert run.get("status") == "success"
        assert isinstance(steps, list) and len(steps) >= 1
        assert any(step.get("node_id") == "sleep-1" and step.get("status") == "success" for step in steps)
    finally:
        response = api_delete(session, gateway_url, f"/api/automation/workflows/{workflow_id}", auth_headers)
        assert response.status_code in (200, 404), response.text