# Object Storage Migration

Homenavi now uses bundled SeaweedFS for S3-compatible profile-picture storage. SeaweedFS does not read MinIO's on-disk format. Preserve existing objects by copying them through the S3 API; do not mount an existing MinIO data directory as SeaweedFS data.

## Docker Compose

The default stack writes new data to the `seaweedfs_data` volume. The old `minio_data` volume remains declared as a migration source and is never mounted by SeaweedFS. MinIO requires a read-write source mount for its own temporary metadata, so stop all application writers and take a volume backup before the copy.

1. Stop `profile-picture-service` to prevent writes during the copy.
2. Start SeaweedFS and initialize its bucket:

   ```sh
   docker compose up -d seaweedfs object-storage-create-bucket
   ```

3. Copy data from the retained MinIO volume:

   ```sh
   docker compose --profile minio-migration run --rm migrate-minio-data
   ```

4. Verify object counts and a sample download with `aws s3 ls` against both endpoints, then start the full stack:

   ```sh
   docker compose up -d
   ```

Do not run `docker compose down --volumes` until the migration is verified. That command removes named volumes, including the legacy source data.

## Helm

Before upgrading an existing MinIO-backed release, retain its old PVC so Helm does not delete it when the SeaweedFS chart removes the MinIO workload. Stop application writers and snapshot the PVC first: MinIO requires read-write access to its own temporary metadata while exposing the legacy objects.

```sh
kubectl -n homenavi annotate pvc homenavi-minio-data \
  helm.sh/resource-policy=keep --overwrite
```

First disable `profile-picture-service`, then upgrade with migration enabled and the retained PVC name:

```sh
helm upgrade --install homenavi ./helm/homenavi -n homenavi \
  --set services.profile-picture-service.enabled=false \
  --set objectStorageMigration.enabled=true \
  --set objectStorageMigration.legacyMinioPVC=homenavi-minio-data
```

The chart starts a temporary legacy MinIO source deployment and an idempotent rclone Job that copies the configured S3 bucket into SeaweedFS. Wait for the Job to complete, verify objects through the SeaweedFS S3 endpoint, then disable migration and re-enable `profile-picture-service`:

```sh
kubectl -n homenavi wait --for=condition=complete job/homenavi-copy-minio-data --timeout=30m
helm upgrade --install homenavi ./helm/homenavi -n homenavi \
  --set objectStorageMigration.enabled=false \
  --set services.profile-picture-service.enabled=true
```

Delete the retained legacy PVC only after a backup and verification.