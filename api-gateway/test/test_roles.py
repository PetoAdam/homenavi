import requests

def _redact_headers(headers: dict):
    if not headers:
        return headers
    redacted = {}
    for k, v in headers.items():
        if k.lower() == "authorization":
            redacted[k] = "<redacted>"
        else:
            redacted[k] = v
    return redacted

def http(method, url, *, headers=None, json=None, params=None):
    print(f"\n==> {method} {url}")
    if params:
        print("Params:", params)
    if headers:
        print("Headers:", _redact_headers(headers))
    if json is not None:
        print("JSON:", json)
    resp = requests.request(method, url, headers=headers, json=json, params=params)
    print(f"<== {resp.status_code} {resp.reason}")
    ct = resp.headers.get("Content-Type", "")
    body = resp.text
    if len(body) > 800:
        print("Body (truncated):", body[:800] + "...")
    else:
        print("Body:", body)
    return resp

GATEWAY = "http://localhost:8080"
AUTH = f"{GATEWAY}/api/auth"
USERS = f"{GATEWAY}/api/users"

def login(email, password):
    r = http("POST", f"{AUTH}/login/start", json={"email": email, "password": password})
    r.raise_for_status()
    data = r.json()
    if "access_token" in data:
        return data["access_token"]
    # if 2FA required, test cannot proceed
    raise RuntimeError("2FA required for test account")

def signup_user(email, username, password):
    r = http("POST", f"{AUTH}/signup", json={
        "user_name": username,
        "email": email,
    "password": password,
    "first_name": "Test",
    "last_name": "User"
    })
    if r.status_code == 201:
        return r.json()["id"]
    if r.status_code in (409, 400):
        # auth-service maps user-service 409 to 400; in either case, fetch by email
        admin_token = login("admin@example.com", "admin")
        rr = http("GET", f"{USERS}", params={"email": email}, headers={"Authorization": f"Bearer {admin_token}"})
        rr.raise_for_status()
        return rr.json()["id"]
    r.raise_for_status()

def test_role_flow():
    admin_token = login("admin@example.com", "admin")
    admin_headers = {"Authorization": f"Bearer {admin_token}"}

    u1_id = signup_user("resident1@example.com", "resident1", "Pass1234A")
    u2_id = signup_user("resident2@example.com", "resident2", "Pass1234A")

    print(f"Assign resident to {u1_id} as admin")
    r = http("PATCH", f"{USERS}/{u1_id}", json={"role": "resident"}, headers=admin_headers)
    assert r.status_code == 200, r.text
    print(f"Assign admin to {u2_id} as admin")
    r = http("PATCH", f"{USERS}/{u2_id}", json={"role": "admin"}, headers=admin_headers)
    assert r.status_code == 200, r.text

    res1_token = login("resident1@example.com", "Pass1234A")
    res1_headers = {"Authorization": f"Bearer {res1_token}"}

    print(f"Resident tries to set {u2_id} to resident")
    r = http("PATCH", f"{USERS}/{u2_id}", json={"role": "resident"}, headers=res1_headers)
    assert r.status_code in (200, 403), r.text

    print(f"Resident tries to set {u1_id} to admin (should be 403)")
    r = http("PATCH", f"{USERS}/{u1_id}", json={"role": "admin"}, headers=res1_headers)
    assert r.status_code == 403, r.text

    print(f"Admin resets {u1_id} to user")
    r = http("PATCH", f"{USERS}/{u1_id}", json={"role": "user"}, headers=admin_headers)
    assert r.status_code == 200, r.text
    print(f"Admin resets {u2_id} to user")
    r = http("PATCH", f"{USERS}/{u2_id}", json={"role": "user"}, headers=admin_headers)
    assert r.status_code == 200, r.text

if __name__ == "__main__":
    test_role_flow()
