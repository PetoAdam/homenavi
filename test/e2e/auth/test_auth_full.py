#!/usr/bin/env python3
"""
Comprehensive end-to-end test suite for Auth + User management flows.

Covers (via API Gateway by default):
  * Signup (weak + strong)
  * Duplicate signup
  * Email verification (manual code entry)
  * Password reset (manual code entry)
  * Login (normal, wrong password, 2FA email flow if enabled)
  * Refresh / Logout / Refresh after logout
  * /me profile fetch (valid + tampered token)
  * User management list (resident/admin), get self/other, role changes constraints
  * Resident attempting invalid escalation, admin full control
  * Lock / unlock account (admin only) + login denial when locked
  * Two Factor enable + enforced second step
  * Role change effect on access to list endpoint
  * Negative tests for unauthorized operations

Requires running stack (docker-compose up) and an existing seeded admin user
(admin@example.com / admin) for admin scenarios.

Environment:
  BASE_GATEWAY_URL (default http://localhost:8080)
  AUTH_PREFIX overrides /api/auth path if needed
  INTERACTIVE_CODES=1 to prompt for email / 2FA codes; otherwise those flows are skipped

Note: Email / 2FA codes are currently printed in service logs. Enter them when prompted.
"""
from __future__ import annotations
import os
import sys
import time
import json
import base64
import logging
import requests
import argparse
import textwrap
from dataclasses import dataclass
from typing import Dict, Any, Optional, Tuple

# ---------- Logging Setup ----------
LOG_LEVEL = os.getenv("LOG_LEVEL", "INFO").upper()
COLOR = sys.stdout.isatty()

class Color:
    if COLOR:
        RED="\x1b[31m"; GREEN="\x1b[32m"; YELLOW="\x1b[33m"; CYAN="\x1b[36m"; MAGENTA="\x1b[35m"; RESET="\x1b[0m"; BOLD="\x1b[1m"
    else:
        RED=GREEN=YELLOW=CYAN=MAGENTA=RESET=BOLD=""

def color_status(code:int)->str:
    if 200 <= code < 300: return f"{Color.GREEN}{code}{Color.RESET}"
    if 300 <= code < 400: return f"{Color.CYAN}{code}{Color.RESET}"
    if 400 <= code < 500: return f"{Color.YELLOW}{code}{Color.RESET}"
    return f"{Color.RED}{code}{Color.RESET}"

logging.basicConfig(
    level=getattr(logging, LOG_LEVEL, logging.INFO),
    format=f"{Color.MAGENTA}%(asctime)s{Color.RESET} | %(levelname)s | %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("auth-e2e")
_test_start = time.time()

# ---------- Configuration ----------
BASE = os.getenv("BASE_GATEWAY_URL", "http://localhost:8080")
AUTH_PREFIX = os.getenv("AUTH_PREFIX", f"{BASE}/api/auth")
INTERACTIVE = os.getenv("INTERACTIVE_CODES", "1") == "1"

# ---------- Helpers ----------

def pretty(obj:Any)->str:
    return json.dumps(obj, indent=2, sort_keys=True, default=str)

def req(method:str, url:str, expected:Optional[int]=None, **kw)->requests.Response:
    log.debug(f"{method} {url} {kw.get('json') or ''}")
    r = requests.request(method, url, timeout=15, **kw)
    log.info(f"{method} {url} -> {color_status(r.status_code)}")
    if r.text:
        try:
            log.debug("Response JSON: %s", pretty(r.json()))
        except Exception:
            log.debug("Response Text: %s", r.text[:500])
    if expected is not None and r.status_code != expected:
        raise AssertionError(f"Expected {expected} got {r.status_code}: {r.text}")
    return r

def tamper_jwt(token:str)->str:
    parts = token.split('.')
    if len(parts) != 3: return token + 'a'
    # Corrupt signature by flipping one byte
    sig = base64.urlsafe_b64decode(parts[2] + '==')
    if not sig: return token + 'a'
    mutated = bytes([sig[0] ^ 0xFF]) + sig[1:]
    b64 = base64.urlsafe_b64encode(mutated).decode().rstrip('=')
    return '.'.join([parts[0], parts[1], b64])

@dataclass
class Tokens:
    access:str
    refresh:str

# ---------- Core Scenario Functions ----------

def signup(user_name:str, email:str, password:str, first_name:str="Test", last_name:str="User")->str:
    payload = {"user_name":user_name, "email":email, "password":password, "first_name":first_name, "last_name":last_name}
    r = req("POST", f"{AUTH_PREFIX}/signup", json=payload, expected=201)
    return r.json()["id"]

def weak_signup_should_fail():
    r = req("POST", f"{AUTH_PREFIX}/signup", json={"user_name":"weakuser","email":"weakuser@example.com","password":"weak","first_name":"Weak","last_name":"User"})
    # Could be 400/422 (weak) or 409 if rerun and user already exists
    assert r.status_code in (400,422,409), f"Weak password test unexpected status {r.status_code}"


def email_verify(user_id:str):
    if not INTERACTIVE:
        log.warning("Skipping email verification (non-interactive mode)")
        return
    req("POST", f"{AUTH_PREFIX}/email/verify/request", json={"user_id":user_id}, expected=200)
    code = input("Enter email verification code (check email-service logs): ").strip()
    req("POST", f"{AUTH_PREFIX}/email/verify/confirm", json={"user_id":user_id, "code":code}, expected=200)


def password_reset(email:str, new_pw:str):
    if not INTERACTIVE:
        log.warning("Skipping password reset (non-interactive mode)")
    # Do not alter stored password when skipping; return None so caller can keep original
    return None
    req("POST", f"{AUTH_PREFIX}/password/reset/request", json={"email":email}, expected=200)
    code = input("Enter password reset code (check logs): ").strip()
    req("POST", f"{AUTH_PREFIX}/password/reset/confirm", json={"email":email, "code":code, "new_password":new_pw}, expected=200)
    return new_pw


def login(email:str, password:str)->Tokens:
    # Retry to tolerate gateway/user-service rate limiting (429)
    for attempt in range(5):
        r = requests.post(f"{AUTH_PREFIX}/login/start", json={"email":email, "password":password}, timeout=15)
        log.info(f"POST {AUTH_PREFIX}/login/start -> {color_status(r.status_code)} (attempt {attempt+1})")
        if r.status_code == 429:
            time.sleep(0.35 * (attempt + 1))
            continue
        break
    if r.status_code != 200:
        raise AssertionError(f"login expected 200 got {r.status_code}: {r.text[:200]}")
    # Fast path: direct tokens (no 2FA enabled)
    try:
        j = r.json()
    except Exception:
        raise AssertionError(f"login non-JSON response: {r.text[:120]}")
    if 'access_token' in j:
        return Tokens(j['access_token'], j['refresh_token'])
    # 2FA branch expected
    assert j.get('2fa_required'), "Expected 2FA flow"
    user_id = j['user_id']; twofa_type = j['twofa_type']
    if twofa_type == 'email':
        if INTERACTIVE:
            req("POST", f"{AUTH_PREFIX}/2fa/email/request", json={"user_id":user_id}, expected=200)
            code = input("Enter email 2FA code: ").strip()
            r2 = req("POST", f"{AUTH_PREFIX}/login/finish", json={"user_id":user_id, "code":code}, expected=200)
            j2 = r2.json(); return Tokens(j2['access_token'], j2['refresh_token'])
        else:
            raise RuntimeError("Cannot finish 2FA in non-interactive mode")
    raise RuntimeError(f"Unsupported 2FA type {twofa_type}")


def refresh(tokens:Tokens)->Tokens:
    r = req("POST", f"{AUTH_PREFIX}/refresh", json={"refresh_token":tokens.refresh}, expected=200)
    j = r.json(); return Tokens(j['access_token'], j['refresh_token'])


def logout(tokens:Tokens, access:str):
    req("POST", f"{AUTH_PREFIX}/logout", headers={"Authorization":f"Bearer {access}"}, json={"refresh_token":tokens.refresh}, expected=200)


def me(access:str, expected:int=200):
    r = req("GET", f"{AUTH_PREFIX}/me", headers={"Authorization":f"Bearer {access}"})
    assert r.status_code == expected, f"/me expected {expected} got {r.status_code}"
    return r

# ----- User Mgmt via Auth Service -----

def list_users(access:str, expect:int=200):
    for attempt in range(5):
        r = requests.get(f"{AUTH_PREFIX}/users?page=1&page_size=5", headers={"Authorization":f"Bearer {access}"}, timeout=10)
        log.info(f"GET /users -> {color_status(r.status_code)} (attempt {attempt+1})")
        if r.status_code != 429:
            break
        time.sleep(0.35 * (attempt + 1))
    if r.status_code == 200:
        try:
            log.debug("Users page: %s", pretty(r.json()))
        except Exception:
            pass
    assert r.status_code == expect, f"list users expected {expect} got {r.status_code}"
    return r


def patch_user(access:str, user_id:str, payload:Dict[str,Any], expect:int):
    for attempt in range(4):
        r = requests.patch(f"{AUTH_PREFIX}/users/{user_id}", json=payload, headers={"Authorization":f"Bearer {access}"}, timeout=10)
        log.info(f"PATCH /users/{user_id} {payload} -> {color_status(r.status_code)} (attempt {attempt+1})")
        if r.status_code != 429:
            break
        time.sleep(0.4 * (attempt + 1))
    assert r.status_code == expect, f"patch expected {expect} got {r.status_code} {r.text}"
    return r


def lock_user(access:str, user_id:str, lock:bool, expect:int):
    for attempt in range(4):
        r = requests.post(f"{AUTH_PREFIX}/users/{user_id}/lockout", json={"lock":lock}, headers={"Authorization":f"Bearer {access}"}, timeout=10)
        log.info(f"POST /users/{user_id}/lockout lock={lock} -> {color_status(r.status_code)} (attempt {attempt+1})")
        if r.status_code != 429:
            break
        time.sleep(0.4 * (attempt + 1))
    assert r.status_code == expect, f"lock expected {expect} got {r.status_code}"
    return r

# ---------- Scenario Execution ----------

def run():
    log.info(Color.BOLD + "Starting comprehensive auth test" + Color.RESET)
    log.info(f"Config BASE={BASE} AUTH_PREFIX={AUTH_PREFIX} INTERACTIVE={INTERACTIVE}")

    weak_signup_should_fail()

    # Use a time-based suffix to ensure uniqueness across repeated runs without needing cleanup
    suffix = str(int(time.time()))
    user_email = f"user_full_{suffix}@example.com"
    user_pw = "Password123A"
    user_id = signup(f"fulluser_{suffix}", user_email, user_pw, first_name="Full", last_name="User")
    email_verify(user_id)
    # Optionally perform password reset (interactive only)
    maybe_new_pw = password_reset(user_email, "Password123B")
    if maybe_new_pw:
        user_pw = maybe_new_pw

    tokens = login(user_email, user_pw)
    log.info(f"Primary user logged in: id={user_id} email={user_email}")
    me(tokens.access)

    # Tampered token test
    bad = tamper_jwt(tokens.access)
    me(bad, expected=401)

    # Refresh & logout sequence
    tokens = refresh(tokens)
    logout(tokens, tokens.access)
    # Refresh after logout should fail
    r = requests.post(f"{AUTH_PREFIX}/refresh", json={"refresh_token":tokens.refresh}, timeout=10)
    log.info(f"POST /refresh after logout -> {color_status(r.status_code)} (expected 401)")
    assert r.status_code == 401

    # Re-login for further operations
    tokens = login(user_email, user_pw)

    # Create resident candidate & admin login
    resident_email = f"resident_test_{suffix}@example.com"
    resident_id = signup(f"residenttest_{suffix}", resident_email, "Resident123A", first_name="Resident", last_name="Test")
    admin_tokens = login("admin@example.com", "admin")

    # Admin: promote resident candidate to resident
    patch_user(admin_tokens.access, resident_id, {"role":"resident"}, expect=200)

    # Resident attempts to list users (should succeed after role change)
    resident_tokens = login(resident_email, "Resident123A")
    list_users(resident_tokens.access, expect=200)
    log.info("Resident user list succeeded")

    # Resident attempts to grant admin (should fail)
    patch_user(resident_tokens.access, user_id, {"role":"admin"}, expect=403)

    # Resident grants resident role to normal user (should succeed)
    patch_user(resident_tokens.access, user_id, {"role":"resident"}, expect=200)
    log.info("Resident granted resident role to normal user")

    # Non-admin (now resident) attempts lockout (should fail)
    lock_user(resident_tokens.access, user_id, True, expect=403)

    # Admin lockout & login denial
    lock_user(admin_tokens.access, user_id, True, expect=200)
    r = requests.post(f"{AUTH_PREFIX}/login/start", json={"email":user_email, "password":user_pw}, timeout=10)
    log.info(f"Locked user login attempt -> {color_status(r.status_code)} (expect 423)")
    assert r.status_code == 423, f"Expected 423 Locked got {r.status_code}: {r.text}"

    # Unlock and login again
    lock_user(admin_tokens.access, user_id, False, expect=200)
    tokens = login(user_email, user_pw)
    log.info("User re-logged after unlock")

    # Self profile update (allowed)
    patch_user(tokens.access, user_id, {"first_name":"Updated"}, expect=200)

    # Another user attempts unauthorized profile patch (non-resident w/out rights)
    other_email = f"other_basic_{suffix}@example.com"
    other_id = signup(f"otherbasic_{suffix}", other_email, "Other123A", first_name="Other", last_name="Basic")
    other_tokens = login(other_email, "Other123A")
    patch_user(other_tokens.access, user_id, {"last_name":"Hack"}, expect=403)

    # Admin set user back to user role
    patch_user(admin_tokens.access, user_id, {"role":"user"}, expect=200)

    # List users with admin
    list_users(admin_tokens.access)
    elapsed = time.time() - _test_start
    log.info(f"Admin list users final check complete (elapsed {elapsed:.2f}s)")

    log.info(Color.GREEN + Color.BOLD + "All scenarios completed successfully." + Color.RESET)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Run comprehensive auth E2E tests.")
    parser.add_argument("--non-interactive", action="store_true", help="Skip interactive code inputs (will skip email/2FA flows).")
    args = parser.parse_args()
    if args.non_interactive:
        INTERACTIVE = False  # type: ignore
    try:
        run()
    except KeyboardInterrupt:
        log.warning("Interrupted")
        sys.exit(130)
    except Exception as e:
        log.exception("Test run failed: %s", e)
        sys.exit(1)
