import os
import time
from urllib.parse import urljoin

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
def gateway_url() -> str:
    return os.getenv("HOMENAVI_GATEWAY_URL", "http://localhost:8080").rstrip("/")


@pytest.fixture(scope="session")
def auth_prefix(gateway_url: str) -> str:
    return urljoin(gateway_url + "/", "api/auth")


@pytest.fixture(scope="session")
def users_prefix(gateway_url: str) -> str:
    return urljoin(gateway_url + "/", "api/users")


@pytest.fixture(scope="session")
def session() -> requests.Session:
    s = requests.Session()
    s.headers.update({"Content-Type": "application/json"})
    return s


@pytest.fixture(scope="session", autouse=True)
def require_gateway(gateway_url: str):
    # Skip the whole suite if the gateway isn't running.
    # This makes these tests safe for CI/unit runs.
    if not _wait_for_reachable(gateway_url):
        pytest.skip(f"API gateway not reachable at {gateway_url}")
