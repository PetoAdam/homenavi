from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol

from botocore.client import Config as BotoConfig
import boto3
from botocore.exceptions import ClientError

from config import Settings


class ObjectNotFoundError(FileNotFoundError):
    pass


@dataclass(frozen=True)
class PreparedUpload:
    object_key: str
    upload_url: str
    method: str
    headers: dict[str, str]
    expires_in: int


@dataclass(frozen=True)
class StoredObject:
    content: bytes
    content_type: str


class StorageBackend(Protocol):
    def put_bytes(self, key: str, content: bytes, content_type: str) -> None:
        ...

    def read_bytes(self, key: str) -> StoredObject:
        ...

    def delete(self, key: str) -> None:
        ...

    def exists(self, key: str) -> bool:
        ...

    def create_upload(self, key: str, content_type: str, expires_in: int) -> PreparedUpload:
        ...

    def create_access_url(self, key: str, expires_in: int) -> str:
        ...


class S3Storage:
    def __init__(self, settings: Settings):
        session = boto3.session.Session()
        self.bucket = settings.s3_bucket or ""
        self.client = session.client(
            "s3",
            endpoint_url=settings.s3_endpoint,
            region_name=settings.s3_region,
            aws_access_key_id=settings.s3_access_key,
            aws_secret_access_key=settings.s3_secret_key,
            config=BotoConfig(s3={"addressing_style": "path" if settings.s3_force_path_style else "auto"}),
        )

    def put_bytes(self, key: str, content: bytes, content_type: str) -> None:
        self.client.put_object(Bucket=self.bucket, Key=key, Body=content, ContentType=content_type)

    def read_bytes(self, key: str) -> StoredObject:
        try:
            response = self.client.get_object(Bucket=self.bucket, Key=key)
        except ClientError as exc:
            if exc.response.get("Error", {}).get("Code") in {"NoSuchKey", "404"}:
                raise ObjectNotFoundError(key) from exc
            raise
        body = response["Body"].read()
        content_type = response.get("ContentType") or "application/octet-stream"
        return StoredObject(content=body, content_type=content_type)

    def delete(self, key: str) -> None:
        self.client.delete_object(Bucket=self.bucket, Key=key)

    def exists(self, key: str) -> bool:
        try:
            self.client.head_object(Bucket=self.bucket, Key=key)
            return True
        except ClientError as exc:
            if exc.response.get("Error", {}).get("Code") in {"404", "NoSuchKey", "NotFound"}:
                return False
            raise

    def create_upload(self, key: str, content_type: str, expires_in: int) -> PreparedUpload:
        url = self.client.generate_presigned_url(
            "put_object",
            Params={"Bucket": self.bucket, "Key": key, "ContentType": content_type},
            ExpiresIn=expires_in,
        )
        return PreparedUpload(object_key=key, upload_url=url, method="PUT", headers={"Content-Type": content_type}, expires_in=expires_in)

    def create_access_url(self, key: str, expires_in: int) -> str:
        return self.client.generate_presigned_url(
            "get_object",
            Params={"Bucket": self.bucket, "Key": key},
            ExpiresIn=expires_in,
        )
