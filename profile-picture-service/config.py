from __future__ import annotations

from dataclasses import dataclass
import os


@dataclass(frozen=True)
class Settings:
    storage_type: str
    s3_endpoint: str | None
    s3_region: str
    s3_bucket: str | None
    s3_access_key: str | None
    s3_secret_key: str | None
    s3_force_path_style: bool
    presign_expiry_seconds: int
    public_base_path: str
    max_upload_bytes: int


class ConfigurationError(ValueError):
    pass


def _get_bool(name: str, default: bool) -> bool:
    value = os.getenv(name)
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}


def load_settings() -> Settings:
    storage_type = os.getenv("STORAGE_TYPE", "s3").strip().lower() or "s3"
    if storage_type != "s3":
        raise ConfigurationError(f"unsupported STORAGE_TYPE {storage_type!r}")

    public_base_path = os.getenv("PROFILE_PICTURE_PUBLIC_BASE_PATH", "/api/profile-pictures").strip() or "/api/profile-pictures"
    if not public_base_path.startswith("/") and not public_base_path.startswith("http://") and not public_base_path.startswith("https://"):
        raise ConfigurationError("PROFILE_PICTURE_PUBLIC_BASE_PATH must be absolute-path or absolute URL")
    public_base_path = public_base_path.rstrip("/")

    settings = Settings(
        storage_type=storage_type,
        s3_endpoint=(os.getenv("STORAGE_S3_ENDPOINT") or "").strip() or None,
        s3_region=os.getenv("STORAGE_S3_REGION", "us-east-1").strip() or "us-east-1",
        s3_bucket=(os.getenv("STORAGE_S3_BUCKET") or "").strip() or None,
        s3_access_key=(os.getenv("STORAGE_S3_ACCESS_KEY") or "").strip() or None,
        s3_secret_key=(os.getenv("STORAGE_S3_SECRET_KEY") or "").strip() or None,
        s3_force_path_style=_get_bool("STORAGE_S3_FORCE_PATH_STYLE", True),
        presign_expiry_seconds=max(int(os.getenv("STORAGE_PRESIGN_EXPIRY_SECONDS", "900")), 60),
        public_base_path=public_base_path,
        max_upload_bytes=max(int(os.getenv("PROFILE_PICTURE_MAX_UPLOAD_BYTES", str(5 * 1024 * 1024))), 1024),
    )

    if settings.storage_type == "s3":
        missing = []
        if not settings.s3_bucket:
            missing.append("STORAGE_S3_BUCKET")
        if not settings.s3_access_key:
            missing.append("STORAGE_S3_ACCESS_KEY")
        if not settings.s3_secret_key:
            missing.append("STORAGE_S3_SECRET_KEY")
        if missing:
            raise ConfigurationError(f"missing S3 configuration: {', '.join(missing)}")

    return settings
