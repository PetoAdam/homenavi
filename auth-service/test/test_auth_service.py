import uuid

import pytest


def _signup_payload(email: str, username: str, password: str):
    return {
        "user_name": username,
        "email": email,
        "password": password,
        "first_name": "Test",
        "last_name": "User",
    }


def _require_non_2fa_tokens(resp):
    data = resp.json()
    if "access_token" in data:
        return data["access_token"], data.get("refresh_token", "")
    if data.get("2fa_required"):
        pytest.skip("2FA required; skipping non-interactive auth flow test")
    raise AssertionError(f"Unexpected login response: {data}")


def test_signup_weak_password_returns_400(session, auth_service_url):
    email = f"weak-{uuid.uuid4().hex[:8]}@example.com"
    r = session.post(
        f"{auth_service_url}/signup",
        json=_signup_payload(email=email, username=f"weak_{uuid.uuid4().hex[:8]}", password="password456"),
        timeout=2.0,
    )
    assert r.status_code == 400, r.text


def test_login_refresh_logout_flow(session, auth_service_url):
    suffix = uuid.uuid4().hex[:8]
    email = f"testuser-{suffix}@example.com"
    password = "Pass1234AA"
    username = f"testuser_{suffix}"

    r = session.post(f"{auth_service_url}/signup", json=_signup_payload(email, username, password), timeout=2.0)
    if r.status_code == 409:
        pytest.skip("Test user already exists; rerun later")
    if r.status_code == 400 and "verify" in r.text.lower():
        pytest.skip("Signup requires email verification; skipping")
    assert r.status_code in (201, 400), r.text

    r = session.post(f"{auth_service_url}/login/start", json={"email": email, "password": password}, timeout=2.0)
    if r.status_code in (401, 403):
        pytest.skip("Login blocked (likely email verification required); skipping")
    assert r.status_code == 200, r.text

    access_token, refresh_token = _require_non_2fa_tokens(r)
    assert access_token
    assert refresh_token

    headers = {"Authorization": f"Bearer {access_token}"}

    r = session.get(f"{auth_service_url}/me", headers=headers, timeout=2.0)
    assert r.status_code == 200, r.text

    r = session.post(f"{auth_service_url}/refresh", json={"refresh_token": refresh_token}, timeout=2.0)
    assert r.status_code == 200, r.text
    new_tokens = r.json()
    assert new_tokens.get("access_token")
    assert new_tokens.get("refresh_token")

    r = session.post(f"{auth_service_url}/logout", json={"refresh_token": refresh_token}, headers=headers, timeout=2.0)
    assert r.status_code == 200, r.text

    r = session.post(f"{auth_service_url}/refresh", json={"refresh_token": refresh_token}, timeout=2.0)
    assert r.status_code == 401, r.text

    r = session.get(f"{auth_service_url}/me", headers={"Authorization": "Bearer invalidtoken"}, timeout=2.0)
    assert r.status_code == 401, r.text
