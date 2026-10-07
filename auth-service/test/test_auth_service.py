import base64
import hashlib
import hmac
import time
import urllib.parse
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
    access_token = resp.cookies.get("auth_token")
    refresh_token = resp.cookies.get("homenavi_refresh_token")
    if access_token and refresh_token:
        return access_token, refresh_token
    if data.get("2fa_required"):
        pytest.skip("2FA required; skipping non-interactive auth flow test")
    raise AssertionError(f"Unexpected login response: {data}")


def _totp(secret: str, timestamp: float | None = None) -> str:
    timestamp = time.time() if timestamp is None else timestamp
    counter = int(timestamp // 30).to_bytes(8, "big")
    padded_secret = secret + "=" * (-len(secret) % 8)
    digest = hmac.new(base64.b32decode(padded_secret, casefold=True), counter, hashlib.sha1).digest()
    offset = digest[-1] & 0x0F
    value = int.from_bytes(digest[offset : offset + 4], "big") & 0x7FFFFFFF
    return f"{value % 1_000_000:06d}"


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
    assert r.cookies.get("auth_token")
    assert r.cookies.get("homenavi_refresh_token")

    r = session.post(f"{auth_service_url}/logout", json={"refresh_token": refresh_token}, headers=headers, timeout=2.0)
    assert r.status_code == 200, r.text

    r = session.post(f"{auth_service_url}/refresh", json={"refresh_token": refresh_token}, timeout=2.0)
    assert r.status_code == 401, r.text

    r = session.get(f"{auth_service_url}/me", headers={"Authorization": "Bearer invalidtoken"}, timeout=2.0)
    assert r.status_code == 401, r.text


def test_totp_enrollment_and_login_flow(session, auth_service_url):
    suffix = uuid.uuid4().hex[:8]
    email = f"totp-{suffix}@example.com"
    password = "Pass1234AA"
    username = f"totp_{suffix}"

    r = session.post(f"{auth_service_url}/signup", json=_signup_payload(email, username, password), timeout=2.0)
    assert r.status_code in (201, 400), r.text

    r = session.post(f"{auth_service_url}/login/start", json={"email": email, "password": password}, timeout=2.0)
    assert r.status_code == 200, r.text
    access_token, refresh_token = _require_non_2fa_tokens(r)
    headers = {"Authorization": f"Bearer {access_token}"}

    r = session.post(f"{auth_service_url}/2fa/setup", json={}, timeout=2.0)
    assert r.status_code == 401, r.text

    r = session.post(f"{auth_service_url}/2fa/setup", json={}, headers=headers, timeout=2.0)
    assert r.status_code == 200, r.text
    enrollment = r.json()
    assert enrollment["otpauth_url"].startswith("otpauth://totp/")
    assert enrollment["qr_code_data_url"].startswith("data:image/png;base64,")
    secret = enrollment["secret"]

    r = session.post(f"{auth_service_url}/2fa/verify", json={"code": "000000"}, headers=headers, timeout=2.0)
    assert r.status_code == 401, r.text

    r = session.post(f"{auth_service_url}/2fa/verify", json={"code": _totp(secret)}, headers=headers, timeout=2.0)
    assert r.status_code == 200, r.text
    verification = r.json()
    assert verification["verified"] is True
    assert len(verification["recovery_codes"]) == 10
    assert len(set(verification["recovery_codes"])) == 10

    r = session.post(f"{auth_service_url}/login/start", json={"email": email, "password": password}, timeout=2.0)
    assert r.status_code == 200, r.text
    login_challenge = r.json()
    assert login_challenge["2fa_required"] is True
    assert login_challenge["2fa_type"] == "totp"

    r = session.post(
        f"{auth_service_url}/login/finish",
        json={"user_id": login_challenge["user_id"], "code": "000000"},
        timeout=2.0,
    )
    assert r.status_code == 401, r.text

    r = session.post(
        f"{auth_service_url}/login/finish",
        json={"user_id": login_challenge["user_id"], "code": _totp(secret)},
        timeout=2.0,
    )
    assert r.status_code == 200, r.text
    assert r.cookies.get("auth_token")
    assert r.cookies.get("homenavi_refresh_token")


def test_oauth_pkce_issues_mcp_only_token(session, auth_service_url, mcp_url):
    public_base_url = mcp_url.removesuffix("/mcp")
    r = session.get(f"{public_base_url}/.well-known/oauth-protected-resource", timeout=2.0)
    assert r.status_code == 200, r.text
    assert r.json()["resource"] == mcp_url

    r = session.get(f"{public_base_url}/.well-known/oauth-authorization-server", timeout=2.0)
    assert r.status_code == 200, r.text
    assert r.json()["issuer"] == f"{public_base_url}/api/auth"

    suffix = uuid.uuid4().hex[:8]
    email = f"oauth-{suffix}@example.com"
    password = "Pass1234AA"
    username = f"oauth_{suffix}"
    r = session.post(f"{auth_service_url}/signup", json=_signup_payload(email, username, password), timeout=2.0)
    assert r.status_code in (201, 400), r.text

    r = session.post(f"{auth_service_url}/login/start", json={"email": email, "password": password}, timeout=2.0)
    assert r.status_code == 200, r.text
    access_token, refresh_token = _require_non_2fa_tokens(r)
    verifier = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
    challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).rstrip(b"=").decode()
    resource = mcp_url
    redirect_uri = "https://client.example/callback"
    authorization_request = {
        "client_id": "compose-mcp-client",
        "redirect_uri": redirect_uri,
        "response_type": "code",
        "scope": "home.devices.read",
        "state": "compose-state",
        "code_challenge": challenge,
        "code_challenge_method": "S256",
        "resource": resource,
    }
    r = session.post(
        f"{auth_service_url}/oauth/consents",
        json=authorization_request,
        headers={"Authorization": f"Bearer {access_token}"},
        timeout=2.0,
    )
    assert r.status_code == 204, r.text
    r = session.get(
        f"{auth_service_url}/oauth/authorize",
        params=authorization_request,
        headers={"Authorization": f"Bearer {access_token}"},
        allow_redirects=False,
        timeout=2.0,
    )
    assert r.status_code == 302, r.text
    redirect = urllib.parse.urlparse(r.headers["Location"])
    values = urllib.parse.parse_qs(redirect.query)
    assert values["state"] == ["compose-state"]
    code = values["code"][0]

    r = session.post(
        f"{auth_service_url}/oauth/token",
        data={
            "grant_type": "authorization_code",
            "code": code,
            "client_id": "compose-mcp-client",
            "redirect_uri": redirect_uri,
            "resource": resource,
            "code_verifier": verifier,
        },
        timeout=2.0,
    )
    assert r.status_code == 200, r.text
    mcp_token = r.json()["access_token"]
    assert r.json()["scope"] == "home.devices.read"

    r = session.get(f"{auth_service_url}/me", headers={"Authorization": f"Bearer {mcp_token}"}, timeout=2.0)
    assert r.status_code == 401, r.text

    mcp_headers = {
        "Authorization": f"Bearer {mcp_token}",
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
    }
    r = session.post(
        mcp_url,
        headers=mcp_headers,
        json={
            "jsonrpc": "2.0",
            "id": 1,
            "method": "initialize",
            "params": {
                "protocolVersion": "2025-03-26",
                "capabilities": {},
                "clientInfo": {"name": "homenavi-compose-test", "version": "1.0"},
            },
        },
        timeout=5.0,
    )
    assert r.status_code == 200, r.text
    assert r.json()["result"]["serverInfo"]["name"] == "homenavi-mcp"

    mcp_headers["MCP-Protocol-Version"] = "2025-03-26"
    r = session.post(mcp_url, headers=mcp_headers, json={"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}}, timeout=5.0)
    assert r.status_code == 200, r.text
    assert "list_devices" in {tool["name"] for tool in r.json()["result"]["tools"]}

    r = session.post(
        mcp_url,
        headers=mcp_headers,
        json={"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "list_devices", "arguments": {}}},
        timeout=5.0,
    )
    assert r.status_code == 200, r.text
    assert isinstance(r.json()["result"]["structuredContent"]["devices"], list)

    r = session.post(f"{auth_service_url}/logout", json={"refresh_token": refresh_token}, headers={"Authorization": f"Bearer {access_token}"}, timeout=2.0)
    assert r.status_code == 200, r.text
    r = session.post(mcp_url, headers=mcp_headers, json={"jsonrpc": "2.0", "id": 4, "method": "tools/list", "params": {}}, timeout=5.0)
    assert r.status_code == 401, r.text
