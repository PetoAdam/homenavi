from __future__ import annotations

import os
import time
import uuid

import pytest
import requests


def _is_reachable(url: str, timeout: float = 0.5) -> bool:
    try:
        response = requests.get(url, timeout=timeout)
        return response.status_code < 500
    except Exception:
        return False


def _wait_for_reachable(url: str, total_timeout: float = 5.0, step: float = 0.25) -> bool:
    deadline = time.time() + total_timeout
    while time.time() < deadline:
        if _is_reachable(url):
            return True
        time.sleep(step)
    return False


@pytest.fixture(scope="session")
def gateway_url() -> str:
    return os.getenv("HOMENAVI_GATEWAY_URL", "http://localhost:8080").rstrip("/")


@pytest.fixture(scope="session")
def smoke_prefix() -> str:
    return os.getenv("HOMENAVI_TEST_PREFIX", f"ha-smoke-{uuid.uuid4().hex[:8]}")


@pytest.fixture(scope="session")
def session() -> requests.Session:
    http_session = requests.Session()
    http_session.headers.update({"Content-Type": "application/json"})
    return http_session


@pytest.fixture(scope="session", autouse=True)
def require_gateway(gateway_url: str):
    if not _wait_for_reachable(gateway_url):
        pytest.skip(f"API gateway not reachable at {gateway_url}")


@pytest.fixture(scope="session")
def admin_token(session: requests.Session, gateway_url: str) -> str:
    response = session.post(
        f"{gateway_url}/api/auth/login/start",
        json={
            "email": os.getenv("HOMENAVI_ADMIN_EMAIL", "admin@example.com"),
            "password": os.getenv("HOMENAVI_ADMIN_PASSWORD", "admin"),
        },
        timeout=10,
    )
    response.raise_for_status()
    payload = response.json()
    if payload.get("2fa_required"):
        pytest.skip("Admin login requires 2FA; HA smoke tests expect a non-interactive admin account")
    token = payload.get("access_token")
    assert token, "admin login did not return access_token"
    return str(token)


@pytest.fixture(scope="session")
def auth_headers(admin_token: str) -> dict[str, str]:
    return {"Authorization": f"Bearer {admin_token}"}