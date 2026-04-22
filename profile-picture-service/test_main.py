from __future__ import annotations

from io import BytesIO
import importlib
import sys
from urllib.parse import urlparse

from PIL import Image
from fastapi.testclient import TestClient


class FakeS3Storage:
    def __init__(self, settings):
        self.settings = settings
        self.objects = {}

    def put_bytes(self, key, content, content_type):
        self.objects[key] = {"content": bytes(content), "content_type": content_type}

    def read_bytes(self, key):
        stored = self.objects.get(key)
        if stored is None:
            raise FileNotFoundError(key)
        from storage import StoredObject

        return StoredObject(content=stored["content"], content_type=stored["content_type"])

    def delete(self, key):
        self.objects.pop(key, None)

    def exists(self, key):
        return key in self.objects

    def create_upload(self, key, content_type, expires_in):
        from storage import PreparedUpload

        return PreparedUpload(
            object_key=key,
            upload_url=f"https://example.invalid/upload/{key}",
            method="PUT",
            headers={"Content-Type": content_type},
            expires_in=expires_in,
        )

    def create_access_url(self, key, expires_in):
        return f"https://example.invalid/access/{key}?expires_in={expires_in}"


def _load_test_client(monkeypatch, tmp_path):
    monkeypatch.setenv("STORAGE_TYPE", "s3")
    monkeypatch.setenv("STORAGE_S3_ENDPOINT", "http://fake-s3.local")
    monkeypatch.setenv("STORAGE_S3_BUCKET", "profile-pictures")
    monkeypatch.setenv("STORAGE_S3_ACCESS_KEY", "test-access")
    monkeypatch.setenv("STORAGE_S3_SECRET_KEY", "test-secret-key")
    monkeypatch.setenv("PROFILE_PICTURE_PUBLIC_BASE_PATH", "/api/profile-pictures")

    storage_module = importlib.import_module("storage")
    monkeypatch.setattr(storage_module, "S3Storage", FakeS3Storage)

    sys.modules.pop("main", None)
    module = importlib.import_module("main")
    module = importlib.reload(module)
    return TestClient(module.app)


def _png_bytes(color=(32, 64, 128)):
    image = Image.new("RGB", (64, 64), color)
    output = BytesIO()
    image.save(output, format="PNG")
    return output.getvalue()


def test_generate_avatar_returns_public_url_and_serves_image(monkeypatch, tmp_path):
    client = _load_test_client(monkeypatch, tmp_path)

    response = client.post("/generate/user-1")
    assert response.status_code == 200
    payload = response.json()
    assert payload["success"] is True
    assert payload["url"].startswith("/api/profile-pictures/users/user-1")

    avatar_response = client.get("/profile-pictures/users/user-1")
    assert avatar_response.status_code == 200
    assert avatar_response.headers["content-type"].startswith("image/png")
    assert avatar_response.content


def test_presigned_upload_flow(monkeypatch, tmp_path):
    client = _load_test_client(monkeypatch, tmp_path)

    upload_url_response = client.post(
        "/profile-pictures/upload-url",
        json={"user_id": "user-2", "filename": "avatar.png", "content_type": "image/png"},
    )
    assert upload_url_response.status_code == 200
    upload_payload = upload_url_response.json()
    assert upload_payload["success"] is True
    assert urlparse(upload_payload["upload_url"]).scheme == "https"

    fake_storage = client.app.state.profile_picture_service.storage
    fake_storage.put_bytes(upload_payload["object_key"], _png_bytes(), "image/png")

    complete_response = client.post(
        "/profile-pictures/complete",
        json={"user_id": "user-2", "object_key": upload_payload["object_key"]},
    )
    assert complete_response.status_code == 200
    complete_payload = complete_response.json()
    assert complete_payload["success"] is True
    assert complete_payload["url"].startswith("/api/profile-pictures/users/user-2")

    avatar_response = client.get("/profile-pictures/users/user-2")
    assert avatar_response.status_code == 200
    assert avatar_response.headers["content-type"].startswith("image/png")
