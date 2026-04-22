from __future__ import annotations

from dataclasses import dataclass
from io import BytesIO
from pathlib import Path
import hashlib
import random
import uuid
from urllib.parse import quote

from PIL import Image, ImageDraw

from config import Settings
from storage import ObjectNotFoundError, PreparedUpload, StorageBackend


ALLOWED_UPLOAD_TYPES = {"image/jpeg", "image/png", "image/jpg", "image/gif", "image/webp"}
MAX_AVATAR_SIZE = 512
DEFAULT_AVATAR_SIZE = 256


@dataclass(frozen=True)
class AvatarResult:
    asset_key: str
    public_url: str
    access_url: str
    version: str


class ProfilePictureService:
    def __init__(self, settings: Settings, storage: StorageBackend):
        self.settings = settings
        self.storage = storage

    def generate_avatar(self, user_id: str, size: int = DEFAULT_AVATAR_SIZE) -> AvatarResult:
        normalized_size = max(64, min(size, MAX_AVATAR_SIZE))
        random_part = uuid.uuid4().hex
        avatar = _generate_pixel_avatar(f"{user_id}_{random_part}", normalized_size)
        return self._store_image(user_id, avatar)

    def prepare_upload(self, user_id: str, filename: str, content_type: str) -> PreparedUpload:
        effective_content_type = (content_type or "application/octet-stream").strip().lower()
        object_key = self._staging_key(user_id, filename)
        return self.storage.create_upload(object_key, effective_content_type, self.settings.presign_expiry_seconds)

    def complete_upload(self, user_id: str, object_key: str) -> AvatarResult:
        stored = self.storage.read_bytes(object_key)
        image = _decode_and_normalize_upload(stored.content, self.settings.max_upload_bytes)
        result = self._store_image(user_id, image)
        self.storage.delete(object_key)
        return result

    def upload_profile_picture(self, user_id: str, content: bytes) -> AvatarResult:
        image = _decode_and_normalize_upload(content, self.settings.max_upload_bytes)
        return self._store_image(user_id, image)

    def get_avatar(self, user_id: str) -> bytes:
        return self.storage.read_bytes(self._avatar_key(user_id)).content

    def get_avatar_content_type(self, user_id: str) -> str:
        return self.storage.read_bytes(self._avatar_key(user_id)).content_type

    def get_access_url(self, user_id: str) -> str:
        key = self._avatar_key(user_id)
        if not self.storage.exists(key):
            raise ObjectNotFoundError(key)
        return self.storage.create_access_url(key, self.settings.presign_expiry_seconds)

    def _store_image(self, user_id: str, image: Image.Image) -> AvatarResult:
        avatar_bytes = _render_avatar_png(image)
        key = self._avatar_key(user_id)
        self.storage.put_bytes(key, avatar_bytes, "image/png")
        version = uuid.uuid4().hex[:12]
        public_url = self._public_url(user_id, version)
        access_url = self.get_access_url(user_id)
        return AvatarResult(asset_key=key, public_url=public_url, access_url=access_url, version=version)

    def _avatar_key(self, user_id: str) -> str:
        return f"avatars/{_safe_key_component(user_id)}/current.png"

    def _staging_key(self, user_id: str, filename: str) -> str:
        suffix = Path(filename or "upload.bin").suffix.lower() or ".bin"
        return f"staging/{_safe_key_component(user_id)}/{uuid.uuid4().hex}{suffix}"

    def _public_url(self, user_id: str, version: str | None = None) -> str:
        encoded_user_id = quote(user_id, safe="")
        url = f"{self.settings.public_base_path}/users/{encoded_user_id}"
        if version:
            url = f"{url}?v={version}"
        return url


def _generate_pixel_avatar(seed: str, size: int = DEFAULT_AVATAR_SIZE) -> Image.Image:
    random.seed(hashlib.md5(seed.encode("utf-8")).hexdigest())
    img = Image.new("RGB", (size, size), (240, 240, 240))
    draw = ImageDraw.Draw(img)
    grid_size = 8
    pixel_size = size // grid_size
    primary_color = (random.randint(50, 200), random.randint(50, 200), random.randint(50, 200))
    secondary_color = (
        min(255, primary_color[0] + 50),
        min(255, primary_color[1] + 50),
        min(255, primary_color[2] + 50),
    )
    for y in range(grid_size):
        for x in range(grid_size // 2):
            if random.random() > 0.5:
                color = primary_color if random.random() > 0.3 else secondary_color
                left_x = x * pixel_size
                left_y = y * pixel_size
                draw.rectangle([left_x, left_y, left_x + pixel_size, left_y + pixel_size], fill=color)
                right_x = (grid_size - 1 - x) * pixel_size
                draw.rectangle([right_x, left_y, right_x + pixel_size, left_y + pixel_size], fill=color)
    return img


def _decode_and_normalize_upload(content: bytes, max_upload_bytes: int) -> Image.Image:
    if len(content) > max_upload_bytes:
        raise ValueError("file too large")
    try:
        image = Image.open(BytesIO(content))
        image.verify()
        image = Image.open(BytesIO(content))
    except Exception as exc:  # noqa: BLE001
        raise ValueError("file is not a valid image") from exc
    if image.mode != "RGB":
        image = image.convert("RGB")
    image.thumbnail((DEFAULT_AVATAR_SIZE, DEFAULT_AVATAR_SIZE), Image.Resampling.LANCZOS)
    square = Image.new("RGB", (DEFAULT_AVATAR_SIZE, DEFAULT_AVATAR_SIZE), (255, 255, 255))
    offset = ((DEFAULT_AVATAR_SIZE - image.width) // 2, (DEFAULT_AVATAR_SIZE - image.height) // 2)
    square.paste(image, offset)
    return square


def _render_avatar_png(image: Image.Image) -> bytes:
    output = BytesIO()
    image.save(output, format="PNG")
    return output.getvalue()


def _safe_key_component(value: str) -> str:
    safe = []
    for char in value.strip():
        if char.isalnum() or char in {"-", "_", "."}:
            safe.append(char)
        else:
            safe.append("-")
    joined = "".join(safe).strip("-")
    return joined or "anonymous"
