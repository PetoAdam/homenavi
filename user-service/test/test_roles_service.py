import requests

USER = "http://localhost:8001"

def test_role_change_rules():
    r = requests.post(f"{USER}/users/00000000-0000-0000-0000-000000000000/role", json={"role": "resident"})
    assert r.status_code in (401, 403, 404)
