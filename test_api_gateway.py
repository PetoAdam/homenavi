import requests
import yaml
import time

GATEWAY_URL = "http://localhost"
ROUTES_CONFIG = "api-gateway/config/routes/routes.yaml"
LOGIN_EMAIL = "test@example.com"
LOGIN_PASSWORD = "password"

# Load routes from YAML
with open(ROUTES_CONFIG) as f:
    routes = yaml.safe_load(f)["routes"]

print("Loaded routes:")
for route in routes:
    print(f"  {route['methods']} {route['path']} (access: {route.get('access', 'public')})")

jwt_token = None
user_id = None

# 1. Test /api/login and get JWT
for route in routes:
    if route["path"] == "/api/login":
        url = GATEWAY_URL + route["path"]
        print(f"\nTesting LOGIN: {route['methods'][0]} {url}")
        resp = requests.post(url, json={"email": LOGIN_EMAIL, "password": LOGIN_PASSWORD})
        print(f"Status: {resp.status_code}")
        print(f"Body: {resp.text}")
        if resp.status_code == 200 and "token" in resp.text:
            jwt_token = resp.json().get("token")
            print(f"Obtained JWT: {jwt_token}")
        break

# 2. Test /api/user/{id} with JWT
for route in routes:
    if "{id}" in route["path"]:
        if not jwt_token:
            print("No JWT available, skipping user endpoint test.")
            continue
        # Use a dummy id or extract from login if available
        test_id = "testid"
        url = GATEWAY_URL + route["path"].replace("{id}", test_id)
        headers = {"Authorization": f"Bearer {jwt_token}"}
        print(f"\nTesting USER: {route['methods'][0]} {url}")
        resp = requests.request(route["methods"][0], url, headers=headers)
        print(f"Status: {resp.status_code}")
        print(f"Body: {resp.text}")
        # Try to extract user_id if present
        try:
            user_id = resp.json().get("user_id")
        except Exception:
            pass

# 3. Test /api/admin/stats with JWT (must be admin)
for route in routes:
    if "/api/admin/stats" in route["path"]:
        if not jwt_token:
            print("No JWT available, skipping admin endpoint test.")
            continue
        url = GATEWAY_URL + route["path"]
        headers = {"Authorization": f"Bearer {jwt_token}"}
        print(f"\nTesting ADMIN: {route['methods'][0]} {url}")
        resp = requests.request(route["methods"][0], url, headers=headers)
        print(f"Status: {resp.status_code}")
        print(f"Body: {resp.text}")

# 4. Test rate limiting (if enabled)
print("\nTesting rate limiting on /api/login...")
for i in range(35):
    resp = requests.post(GATEWAY_URL + "/api/login", json={"email": LOGIN_EMAIL, "password": LOGIN_PASSWORD})
    print(f"Attempt {i+1}: Status {resp.status_code}")
    time.sleep(0.1)

# 5. Test WebSocket echo endpoint with JWT
try:
    import asyncio
    import websockets
except ImportError:
    print("websockets library not installed, skipping WebSocket test. Run 'pip install websockets' to enable.")
else:
    async def test_ws_echo():
        if not jwt_token:
            print("No JWT available, skipping WebSocket test.")
            return
        ws_url = "ws://localhost/ws/echo"
        print(f"\nTesting WebSocket echo at {ws_url} with Authorization header")
        try:
            headers = {"Authorization": f"Bearer {jwt_token}"}
            async with websockets.connect(ws_url, extra_headers=headers) as websocket:
                await websocket.send("hello via ws")
                response = await websocket.recv()
                print(f"WebSocket response: {response}")
        except Exception as e:
            print(f"WebSocket test failed: {e}")

    asyncio.run(test_ws_echo())
