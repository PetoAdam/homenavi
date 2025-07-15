import requests
import json
import time

AUTH_SERVICE_URL = "http://localhost:8000/api/auth"

print("\n--- AUTH SERVICE INTEGRATION TEST ---")

def print_result(r):
    print(f"Status: {r.status_code}")
    print(f"Body: {r.text}")

def expect_status(r, expected):
    assert r.status_code == expected, f"Expected {expected}, got {r.status_code}: {r.text}"

# Signup with weak password
print("\nTesting signup...")
signup_data = {
    "user_name": "testuser2",
    "email": "testuser2@example.com",
    "password": "password456"
}
r = requests.post(f"{AUTH_SERVICE_URL}/signup", json=signup_data)
print_result(r)
expect_status(r, 400)  # Expect weak password error
if r.status_code != 400:
    exit(1)

# Signup
print("\nTesting signup...")
signup_data = {
    "user_name": "testuser2",
    "email": "testuser2@example.com",
    "password": "password456A"
}
r = requests.post(f"{AUTH_SERVICE_URL}/signup", json=signup_data)
print_result(r)
user = r.json() if r.status_code == 201 else None
user_id = user.get("id") if user else None
if not user_id:
    print("Signup failed, aborting.")
    exit(1)

# Email verification request
print("\nTesting email verification request...")
r = requests.post(f"{AUTH_SERVICE_URL}/email/verify/request", json={"user_id": user_id})
print_result(r)
code = input("Enter email verification code (check logs): ")
r = requests.post(f"{AUTH_SERVICE_URL}/email/verify/confirm", json={"user_id": user_id, "code": code})
print_result(r)

# Password reset request
print("\nTesting password reset request...")
r = requests.post(f"{AUTH_SERVICE_URL}/password/reset/request", json={"email": "testuser2@example.com"})
print_result(r)
code = input("Enter password reset code (check logs): ")
new_pass = input("Enter new password: ")
r = requests.post(f"{AUTH_SERVICE_URL}/password/reset/confirm", json={"email": "testuser2@example.com", "code": code, "new_password": new_pass})
print_result(r)

# Login step 1 (no 2FA)
print("\nTesting login step 1 (no 2FA)...")
login_data = {
    "email": "testuser2@example.com",
    "password": new_pass
}
r = requests.post(f"{AUTH_SERVICE_URL}/login/start", json=login_data)
print_result(r)
if r.status_code == 200 and "access_token" in r.text:
    tokens = r.json()
    access_token = tokens["access_token"]
    refresh_token = tokens["refresh_token"]
else:
    # 2FA required
    resp = r.json()
    assert resp.get("2fa_required"), "Expected 2fa_required in response"
    user_id = resp["user_id"]
    twofa_type = resp["2fa_type"]
    # Simulate email 2FA
    if twofa_type == "email":
        print("\nTesting email 2FA request...")
        r2 = requests.post(f"{AUTH_SERVICE_URL}/2fa/email/request", json={"user_id": user_id})
        print_result(r2)
        code = input("Enter email 2FA code (check logs): ")
        print("\nTesting login step 2 (2FA)...")
        r3 = requests.post(f"{AUTH_SERVICE_URL}/login/finish", json={"user_id": user_id, "code": code})
        print_result(r3)
        tokens = r3.json()
        access_token = tokens["access_token"]
        refresh_token = tokens["refresh_token"]
    else:
        print(f"2FA type {twofa_type} not automated in test.")
        exit(1)

# After login, use headers = {"Authorization": f"Bearer {access_token}"} for all protected requests
headers = {"Authorization": f"Bearer {access_token}"}

# Test /me endpoint
print("\nTesting /me endpoint...")
r = requests.get(f"{AUTH_SERVICE_URL}/me", headers=headers)
print_result(r)
expect_status(r, 200)

# Test refresh token
print("\nTesting refresh token...")
r = requests.post(f"{AUTH_SERVICE_URL}/refresh", json={"refresh_token": refresh_token})
print_result(r)
expect_status(r, 200)
new_tokens = r.json()
access_token = new_tokens["access_token"]
refresh_token = new_tokens["refresh_token"]

# Test logout
print("\nTesting logout...")
r = requests.post(f"{AUTH_SERVICE_URL}/logout", json={"refresh_token": refresh_token}, headers=headers)
print_result(r)
expect_status(r, 200)

# Test refresh after logout (should fail)
print("\nTesting refresh after logout (should fail)...")
r = requests.post(f"{AUTH_SERVICE_URL}/refresh", json={"refresh_token": refresh_token})
print_result(r)
assert r.status_code == 401

# Test lockout (admin only, should fail as non-admin)
print("\nTesting lockout (should fail as non-admin)...")
USER_SERVICE_URL = "http://localhost:8001"
r = requests.post(f"{USER_SERVICE_URL}/users/{user_id}/lockout", json={"lock": True}, headers=headers)
print_result(r)
assert r.status_code == 403

# Test /me with invalid token
print("\nTesting /me with invalid token...")
r = requests.get(f"{AUTH_SERVICE_URL}/me", headers={"Authorization": "Bearer invalidtoken"})
print_result(r)
assert r.status_code == 401

# Test login with wrong password
print("\nTesting login with wrong password...")
login_data = {
    "email": "testuser2@example.com",
    "password": "wrongpassword"
}
r = requests.post(f"{AUTH_SERVICE_URL}/login/start", json=login_data)
print_result(r)
assert r.status_code == 401

# Test signup with duplicate email
print("\nTesting signup with duplicate email...")
r = requests.post(f"{AUTH_SERVICE_URL}/signup", json=signup_data)
print_result(r)
assert r.status_code == 409

# --- Email 2FA setup and login ---
print("\nTesting signup with email 2FA...")
user2_data = {
    "user_name": "testuser2fa",
    "email": "testuser2fa@example.com",
    "password": "password789A"
}
r = requests.post(f"{AUTH_SERVICE_URL}/signup", json=user2_data)
print_result(r)
user2 = r.json() if r.status_code == 201 else None
user2_id = user2.get("id") if user2 else None
if not user2_id:
    print("Signup failed for 2FA user, aborting.")
    exit(1)

# Login as user2 to get JWT
print("\nLogging in as testuser2fa before enabling email 2FA...")
login_data = {
    "email": "testuser2fa@example.com",
    "password": "password789A"
}
r = requests.post(f"{AUTH_SERVICE_URL}/login/start", json=login_data)
print_result(r)
if r.status_code == 200 and "access_token" in r.text:
    tokens2 = r.json()
    access_token2 = tokens2["access_token"]
    headers2 = {"Authorization": f"Bearer {access_token2}"}
else:
    print("Login failed for testuser2fa, aborting.")
    exit(1)

print("\nEnabling email 2FA for testuser2fa...")
r = requests.post(f"{AUTH_SERVICE_URL}/2fa/email/request", json={"user_id": user2_id}, headers=headers2)
print_result(r)
code = input("Enter email 2FA code (check logs): ")
r = requests.post(f"{AUTH_SERVICE_URL}/2fa/email/verify", json={"user_id": user2_id, "code": code}, headers=headers2)
print_result(r)

# Login with email 2FA
print("\nTesting login with email 2FA...")
login_data = {
    "email": "testuser2fa@example.com",
    "password": "password789A"
}
r = requests.post(f"{AUTH_SERVICE_URL}/login/start", json=login_data)
print_result(r)
resp = r.json()
if resp.get("2fa_required"):
    user2_id = resp["user_id"]
    twofa_type = resp["2fa_type"]
    if twofa_type == "email":
        print("\nRequesting email 2FA code...")
        r2 = requests.post(f"{AUTH_SERVICE_URL}/2fa/email/request", json={"user_id": user2_id}, headers=headers2)
        print_result(r2)
        code = input("Enter email 2FA code (check logs): ")
        print("\nFinishing login with email 2FA...")
        r3 = requests.post(f"{AUTH_SERVICE_URL}/login/finish", json={"user_id": user2_id, "code": code})
        print_result(r3)
        tokens2 = r3.json()
        access_token2 = tokens2["access_token"]
        refresh_token2 = tokens2["refresh_token"]
        print("\nTesting /me endpoint for 2FA user...")
        headers2 = {"Authorization": f"Bearer {access_token2}"}
        r = requests.get(f"{AUTH_SERVICE_URL}/me", headers=headers2)
        print_result(r)
        expect_status(r, 200)
else:
    print("Email 2FA not required for testuser2fa, aborting.")
    exit(1)
    
# Delete user at the end (should fail as non-admin)
print("\nDeleting test user (should fail as non-admin)...")
r = requests.post(f"{AUTH_SERVICE_URL}/delete", json={"user_id": user2_id}, headers=headers)
print_result(r)

# --- Admin login and delete test user ---
print("\nTesting admin login...")
admin_login = {
    "email": "admin@example.com",
    "password": "admin"
}
r = requests.post(f"{AUTH_SERVICE_URL}/login/start", json=admin_login)
print_result(r)
if r.status_code == 200 and "access_token" in r.text:
    admin_tokens = r.json()
    admin_access_token = admin_tokens["access_token"]
    admin_headers = {"Authorization": f"Bearer {admin_access_token}"}
    print("\nAdmin deleting test user...")
    r = requests.post(f"{AUTH_SERVICE_URL}/delete", json={"user_id": user_id}, headers=admin_headers)
    print_result(r)
    print("\nAdmin deleting test user 2...")
    r = requests.post(f"{AUTH_SERVICE_URL}/delete", json={"user_id": user2_id}, headers=admin_headers)
    print_result(r)
else:
    print("Admin login failed, aborting.")
    exit(1)

print("\n--- END OF TEST ---")
