from __future__ import annotations

import logging
from typing import Any

from fastapi import FastAPI, File, Form, HTTPException, Response, UploadFile
from fastapi.responses import JSONResponse

from config import ConfigurationError, Settings, load_settings
from service import ALLOWED_UPLOAD_TYPES, ProfilePictureService
from storage import ObjectNotFoundError, S3Storage

logger = logging.getLogger(__name__)


def _build_app() -> FastAPI:
    settings = load_settings()
    storage = S3Storage(settings)
    service = ProfilePictureService(settings, storage)

    app = FastAPI(title="Profile Picture Generator", version="2.0.0")
    app.state.settings = settings
    app.state.profile_picture_service = service

    @app.get("/")
    async def root() -> dict[str, Any]:
        return {
            "message": "Profile Picture Generator Service",
            "version": "2.0.0",
            "storage_type": settings.storage_type,
        }

    @app.post("/generate/{user_id}")
    async def generate_avatar(user_id: str, size: int = 256) -> dict[str, Any]:
        try:
            result = service.generate_avatar(user_id, size)
        except Exception as exc:  # noqa: BLE001
            logger.exception("avatar generation failed", extra={"user_id": user_id})
            raise HTTPException(status_code=500, detail=f"Avatar generation failed: {exc}") from exc
        return {
            "success": True,
            "asset_key": result.asset_key,
            "url": result.public_url,
            "access_url": result.access_url,
            "size": size,
        }

    @app.post("/upload")
    async def upload_profile_picture(user_id: str = Form(...), file: UploadFile = File(...)) -> dict[str, Any]:
        content_type = (file.content_type or "").lower()
        if content_type and content_type not in ALLOWED_UPLOAD_TYPES:
            raise HTTPException(status_code=400, detail=f"File must be an image (got content_type={file.content_type})")
        content = await file.read()
        try:
            result = service.upload_profile_picture(user_id, content)
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc
        except Exception as exc:  # noqa: BLE001
            logger.exception("upload failed", extra={"user_id": user_id})
            raise HTTPException(status_code=500, detail=f"Upload failed: {exc}") from exc
        return {
            "success": True,
            "asset_key": result.asset_key,
            "url": result.public_url,
            "access_url": result.access_url,
            "primary_url": result.public_url,
        }

    @app.post("/profile-pictures/upload-url")
    async def create_upload_url(payload: dict[str, Any]) -> dict[str, Any]:
        user_id = str(payload.get("user_id", "")).strip()
        filename = str(payload.get("filename", "")).strip()
        content_type = str(payload.get("content_type", "")).strip().lower()
        if not user_id or not filename:
            raise HTTPException(status_code=400, detail="user_id and filename are required")
        if content_type and content_type not in ALLOWED_UPLOAD_TYPES:
            raise HTTPException(status_code=400, detail="unsupported content type")
        prepared = service.prepare_upload(user_id, filename, content_type or "application/octet-stream")
        return {
            "success": True,
            "object_key": prepared.object_key,
            "upload_url": prepared.upload_url,
            "method": prepared.method,
            "headers": prepared.headers,
            "expires_in": prepared.expires_in,
        }

    @app.post("/profile-pictures/complete")
    async def complete_upload(payload: dict[str, Any]) -> dict[str, Any]:
        user_id = str(payload.get("user_id", "")).strip()
        object_key = str(payload.get("object_key", "")).strip()
        if not user_id or not object_key:
            raise HTTPException(status_code=400, detail="user_id and object_key are required")
        try:
            result = service.complete_upload(user_id, object_key)
        except ObjectNotFoundError as exc:
            raise HTTPException(status_code=404, detail="uploaded object not found") from exc
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc
        except Exception as exc:  # noqa: BLE001
            logger.exception("upload completion failed", extra={"user_id": user_id, "object_key": object_key})
            raise HTTPException(status_code=500, detail=f"Upload completion failed: {exc}") from exc
        return {
            "success": True,
            "asset_key": result.asset_key,
            "url": result.public_url,
            "access_url": result.access_url,
        }

    @app.get("/profile-pictures/users/{user_id}")
    async def get_avatar(user_id: str) -> Response:
        try:
            content = service.get_avatar(user_id)
            content_type = service.get_avatar_content_type(user_id)
        except ObjectNotFoundError as exc:
            raise HTTPException(status_code=404, detail="avatar not found") from exc
        return Response(content=content, media_type=content_type, headers={"Cache-Control": "no-cache, no-store, must-revalidate"})

    @app.get("/profile-pictures/users/{user_id}/access-url")
    async def get_avatar_access_url(user_id: str) -> dict[str, Any]:
        try:
            url = service.get_access_url(user_id)
        except ObjectNotFoundError as exc:
            raise HTTPException(status_code=404, detail="avatar not found") from exc
        return {"success": True, "url": url, "expires_in": settings.presign_expiry_seconds}

    @app.get("/health")
    async def health_check() -> dict[str, Any]:
        return {"status": "healthy", "storage_type": settings.storage_type}

    return app


try:
    app = _build_app()
except ConfigurationError as exc:
    logger.exception("profile picture service configuration error")
    config_error_message = str(exc)
    error_app = FastAPI(title="Profile Picture Generator", version="2.0.0")

    @error_app.get("/health")
    async def broken_health() -> JSONResponse:
        return JSONResponse(status_code=500, content={"status": "broken", "error": config_error_message})

    app = error_app