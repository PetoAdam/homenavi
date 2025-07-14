import requests
import yaml
import time
import glob
import os

GATEWAY_URL = "http://localhost"
ROUTES_DIR = "api-gateway/config/routes"
LOGIN_EMAIL = "test@example.com"
LOGIN_PASSWORD = "password"

# Load all routes from all YAML files in ROUTES_DIR
def load_all_routes():
    routes = []
    for yaml_file in glob.glob(os.path.join(ROUTES_DIR, "*.yaml")):
        with open(yaml_file) as f:
            data = yaml.safe_load(f)
            if not data or not isinstance(data, dict):
                continue
            route_list = data.get("routes")
            if not route_list or not isinstance(route_list, list):
                continue
            routes.extend(route_list)
    return routes

routes = load_all_routes()

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

# 2. Test all REST endpoints with JWT if required
for route in routes:
    if route.get("type", "rest") == "rest" and route["path"] != "/api/login":
        url = GATEWAY_URL + route["path"]
        method = route["methods"][0]
        headers = {}
        if route.get("access") in ["auth", "admin"] and jwt_token:
            headers["Authorization"] = f"Bearer {jwt_token}"
        print(f"\nTesting REST: {method} {url}")
        resp = requests.request(method, url, headers=headers)
        print(f"Status: {resp.status_code}")
        print(f"Body: {resp.text}")

# 3. Test all WebSocket endpoints with JWT if required
try:
    import asyncio
    import websockets
except ImportError:
    print("websockets library not installed, skipping WebSocket test. Run 'pip install websockets' to enable.")
else:
    async def test_all_ws():
        for route in routes:
            if route.get("type") == "websocket":
                ws_url = "ws://localhost" + route["path"]
                headers = {}
                if route.get("access") in ["auth", "admin"] and jwt_token:
                    headers["Authorization"] = f"Bearer {jwt_token}"
                print(f"\nTesting WebSocket at {ws_url} with Authorization header")
                try:
                    async with websockets.connect(ws_url, extra_headers=headers) as websocket:
                        await websocket.send("hello via ws")
                        response = await websocket.recv()
                        print(f"WebSocket response: {response}")
                except Exception as e:
                    print(f"WebSocket test failed: {e}")
    asyncio.run(test_all_ws())

# 4. Test overlapping routes: /api/users/{id} vs /api/users/*
print("\nTesting overlapping routes:")
specific_path = "/api/users/testid"
headers = {"Authorization": f"Bearer {jwt_token}"} if jwt_token else {}
resp = requests.get(GATEWAY_URL + specific_path, headers=headers)
print(f"Request to {specific_path}: Status {resp.status_code}")
print(f"Body: {resp.text}")

wildcard_path = "/api/users/validate"
resp = requests.get(GATEWAY_URL + wildcard_path, headers=headers)
print(f"Request to {wildcard_path}: Status {resp.status_code}")
print(f"Body: {resp.text}")

print("\nCheck your API Gateway logs for 'Proxying ...' to see which upstream was used for each request.")
