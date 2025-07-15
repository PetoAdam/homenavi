import requests
import json

USER_SERVICE_URL = "http://localhost:8001"

# Test user creation
print("\nTesting user creation...")
user_data = {
    "user_name": "testuser",
    "email": "testuser@example.com",
    "password": "password123"
}
resp = requests.post(f"{USER_SERVICE_URL}/user", json=user_data)
print(f"Status: {resp.status_code}")
print(f"Body: {resp.text}")
if resp.status_code == 201:
    user = resp.json()
    user_id = user.get("id")
else:
    print("User creation failed, skipping further tests.")
    exit(1)

# Test get user
print("\nTesting get user...")
resp = requests.get(f"{USER_SERVICE_URL}/user/{user_id}")
print(f"Status: {resp.status_code}")
print(f"Body: {resp.text}")

# Test send email verification (simulate trigger)
print("\nTesting send email verification...")
resp = requests.post(f"{USER_SERVICE_URL}/user/email/send-verification", json={"user_id": user_id})
print(f"Status: {resp.status_code}")
print(f"Body: {resp.text}")

# Test email verification
print("\nTesting email verification...")
verification_code = input("Provide the verification code sent to your email and press Enter...")
resp = requests.post(f"{USER_SERVICE_URL}/user/email/verify", json={"user_id": user_id, "code": verification_code})
print(f"Status: {resp.status_code}")
print(f"Body: {resp.text}")

# Test 2FA setup (simulate trigger)
print("\nTesting 2FA setup...")
resp = requests.post(f"{USER_SERVICE_URL}/user/2fa/setup", json={"user_id": user_id, "method": "totp"})
print(f"Status: {resp.status_code}")
print(f"Body: {resp.text}")

# Test 2FA verify
print("\nTesting 2FA verify...")
resp = requests.post(f"{USER_SERVICE_URL}/user/2fa/verify", json={"user_id": user_id, "code": "654321", "method": "totp"})
print(f"Status: {resp.status_code}")
print(f"Body: {resp.text}")

# Test 2FA disable
print("\nTesting 2FA disable...")
resp = requests.post(f"{USER_SERVICE_URL}/user/2fa/disable", json={"user_id": user_id, "method": "totp"})
print(f"Status: {resp.status_code}")
print(f"Body: {resp.text}")

# Test lockout
print("\nTesting lockout...")
resp = requests.post(f"{USER_SERVICE_URL}/user/lockout", json={"user_id": user_id, "lock": True})
print(f"Status: {resp.status_code}")
print(f"Body: {resp.text}")

# Test delete user
print("\nTesting delete user...")
resp = requests.delete(f"{USER_SERVICE_URL}/user/{user_id}")
print(f"Status: {resp.status_code}")
print(f"Body: {resp.text}")
