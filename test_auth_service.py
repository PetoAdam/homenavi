import requests
import json
import time

AUTH_SERVICE_URL = "http://localhost:8000"

print("\n--- AUTH SERVICE INTEGRATION TEST ---")

# Signup
print("\nTesting signup...")
signup_data = {
    "user_name": "testuser2",
    "email": "testuser2@example.com",
    "password": "password456"
}
r = requests.post(f"{AUTH_SERVICE_URL}/signup", json=signup_data)
print(f"Status: {r.status_code}")
print(f"Body: {r.text}")
user = r.json() if r.status_code == 201 else None
user_id = user.get("id") if user else None
if not user_id:
    print("Signup failed, aborting.")
    exit(1)

# Email verification request
print("\nTesting email verification request...")
r = requests.post(f"{AUTH_SERVICE_URL}/email/verify/request", json={"user_id": user_id})
print(f"Status: {r.status_code}")
print(f"Body: {r.text}")
code = input("Enter email verification code (check logs): ")
r = requests.post(f"{AUTH_SERVICE_URL}/email/verify/confirm", json={"user_id": user_id, "code": code})
print(f"Status: {r.status_code}")
print(f"Body: {r.text}")

# Password reset request
print("\nTesting password reset request...")
r = requests.post(f"{AUTH_SERVICE_URL}/password/reset/request", json={"email": "testuser2@example.com"})
print(f"Status: {r.status_code}")
print(f"Body: {r.text}")
code = input("Enter password reset code (check logs): ")
new_pass = input("Enter new password: ")
r = requests.post(f"{AUTH_SERVICE_URL}/password/reset/confirm", json={"email": "testuser2@example.com", "code": code, "new_password": new_pass})
print(f"Status: {r.status_code}")
print(f"Body: {r.text}")

# Login with new password (no 2FA)
print("\nTesting login with new password...")
login_data = {
    "email": "testuser2@example.com",
    "password": new_pass,
    "code": "" # No 2FA for now
}
r = requests.post(f"{AUTH_SERVICE_URL}/login", json=login_data)
print(f"Status: {r.status_code}")
print(f"Body: {r.text}")

# Email 2FA request
print("\nTesting email 2FA request...")
r = requests.post(f"{AUTH_SERVICE_URL}/2fa/email/request", json={"user_id": user_id})
print(f"Status: {r.status_code}")
print(f"Body: {r.text}")
code = input("Enter email 2FA code (check logs): ")
r = requests.post(f"{AUTH_SERVICE_URL}/2fa/email/verify", json={"user_id": user_id, "code": code})
print(f"Status: {r.status_code}")
print(f"Body: {r.text}")

# Login with email 2FA
print("\nTesting login with email 2FA...")
login_data = {
    "email": "testuser2@example.com",
    "password": new_pass,
    "code": code
}
r = requests.post(f"{AUTH_SERVICE_URL}/login", json=login_data)
print(f"Status: {r.status_code}")
print(f"Body: {r.text}")

# --- TOTP 2FA QR code (future) ---
# To test TOTP 2FA, use the /2fa/setup endpoint and display the otpauth_url as a QR code:
# import qrcode
# resp = requests.get(f"{AUTH_SERVICE_URL}/2fa/setup?user_id={user_id}").json()
# print(f"TOTP Secret: {resp['secret']}")
# print(f"TOTP otpauth URL: {resp['otpauth_url']}")
# qrcode.make(resp["otpauth_url"]).show()
# Then scan with Google Authenticator and use the code in /2fa/verify and /login

# Delete user at the end
print("\nDeleting test user...")
r = requests.post(f"{AUTH_SERVICE_URL}/delete", json={"user_id": user_id})
print(f"Status: {r.status_code}")
print(f"Body: {r.text}")

print("\n--- END OF TEST ---")
