from __future__ import annotations

from io import BytesIO
import importlib
import sys
import threading
from urllib.parse import urlparse

from PIL import Image
from fastapi.testclient import TestClient
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter


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


def test_request_continues_trace_without_recording_user_data(monkeypatch, tmp_path):
    monkeypatch.delenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", raising=False)
    client = _load_test_client(monkeypatch, tmp_path)
    exporter = InMemorySpanExporter()
    client.app.state.tracer_provider.add_span_processor(SimpleSpanProcessor(exporter))
    response = client.post(
        "/generate/secret-user?token=secret-value",
        headers={"traceparent": "00-11111111111111111111111111111111-2222222222222222-01", "baggage": "token=secret"},
    )
    assert response.status_code == 200
    spans = exporter.get_finished_spans()
    assert len(spans) == 1
    span = spans[0]
    assert format(span.context.trace_id, "032x") == "11111111111111111111111111111111"
    assert format(span.parent.span_id, "016x") == "2222222222222222"
    assert response.headers["Trace-ID"] == "11111111111111111111111111111111"
    assert "secret" not in str(span.attributes)
    assert not span.events


def test_exporter_setup_failure_does_not_prevent_requests(monkeypatch, tmp_path):
    monkeypatch.setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "https://collector.example/v1/traces")

    def fail_exporter(*args, **kwargs):
        raise ValueError("collector credentials")

    monkeypatch.setattr("tracing.OTLPSpanExporter", fail_exporter)
    client = _load_test_client(monkeypatch, tmp_path)
    assert client.post("/generate/user-1").status_code == 200


def test_slow_tracing_shutdown_does_not_block_service(monkeypatch, tmp_path):
    monkeypatch.setattr("tracing.TRACING_SHUTDOWN_TIMEOUT", 0.01)
    client = _load_test_client(monkeypatch, tmp_path)
    release = threading.Event()
    finished = threading.Event()

    def slow_shutdown():
        release.wait()
        finished.set()

    monkeypatch.setattr(client.app.state.tracer_provider, "shutdown", slow_shutdown)
    try:
        with client:
            assert client.get("/health").status_code == 200
        assert not finished.is_set()
    finally:
        release.set()
        assert finished.wait(1)


def test_invalid_trace_and_error_response_are_redacted(monkeypatch, tmp_path):
    monkeypatch.delenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", raising=False)
    client = _load_test_client(monkeypatch, tmp_path)
    exporter = InMemorySpanExporter()
    client.app.state.tracer_provider.add_span_processor(SimpleSpanProcessor(exporter))

    def fail_upload(*args, **kwargs):
        raise RuntimeError("secret-storage-error")

    monkeypatch.setattr(client.app.state.profile_picture_service.storage, "put_bytes", fail_upload)
    response = client.post("/generate/secret-user", headers={"traceparent": "invalid"})
    assert response.status_code == 500
    span = exporter.get_finished_spans()[0]
    assert span.parent is None
    assert span.status.description == "HTTP server error"
    assert "secret" not in str(span.attributes)
    assert not span.events


def test_configuration_error_health_is_traced(monkeypatch):
    monkeypatch.delenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", raising=False)
    monkeypatch.setenv("STORAGE_TYPE", "unsupported")
    module = importlib.reload(importlib.import_module("main"))
    exporter = InMemorySpanExporter()
    module.app.state.tracer_provider.add_span_processor(SimpleSpanProcessor(exporter))
    response = TestClient(module.app).get("/health")
    assert response.status_code == 500
    assert exporter.get_finished_spans()[0].status.description == "HTTP server error"


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
