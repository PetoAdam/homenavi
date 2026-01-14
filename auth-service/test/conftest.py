import os
import time

import pytest
import requests


def _is_reachable(url: str, timeout: float = 0.5) -> bool:
    try:
        r = requests.get(url, timeout=timeout)
        return r.status_code < 500
    except Exception:
        return False


def _wait_for_reachable(url: str, total_timeout: float = 2.0, step: float = 0.25) -> bool:
    deadline = time.time() + total_timeout
    while time.time() < deadline:
        if _is_reachable(url):
            return True
        time.sleep(step)
    return False


@pytest.fixture(scope="session")
def auth_service_url() -> str:
    # Direct auth-service endpoint (not via gateway)
    return os.getenv("HOMENAVI_AUTH_SERVICE_URL", "http://localhost:8000/api/auth").rstrip("/")


@pytest.fixture(scope="session")
def session() -> requests.Session:
    s = requests.Session()
    s.headers.update({"Content-Type": "application/json"})
    return s


@pytest.fixture(scope="session", autouse=True)
def require_auth_service(auth_service_url: str):
    # Skip the whole suite if auth-service isn't running.
    if not _wait_for_reachable(auth_service_url):
        pytest.skip(f"Auth service not reachable at {auth_service_url}")
