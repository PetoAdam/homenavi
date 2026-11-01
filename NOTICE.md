# Third-party notices

Homenavi is distributed with third-party software and container images. Their
licenses and notices remain applicable. Generated dependency inventories are
published by the license-scanning workflows and included in frontend build
outputs where applicable.

## Runtime dependencies

- Eclipse Mosquitto is dual-licensed under EPL-2.0 and EDL-1.0.
- Valkey is licensed under BSD-3-Clause and is used through the Redis protocol.
- The optional EMQX provider is BSL 1.1. Homenavi supports it only as an
  explicit single-node compatibility profile; do not enable EMQX clustering
  without a separate commercial-license review.
- The optional VerneMQ HA provider uses the digest-pinned
  `ghcr.io/micw/vernemq-docker@sha256:8fb3b4e2bef3b2700a965477ab67b8122ccb36d06aa6a9920d2f23c7293028a7`
  community-built Apache-2.0 image project. Its SBOM and vulnerability status
  must be reviewed before production rollout.

## Generated inventories

- Go modules: `go-licenses csv` runs once per `go.mod` through
  `scripts/generate-go-license-inventory.sh`.
- The Vite frontend emits `dist/third-party-notices.txt` through
  `vite-plugin-license`.
- The public Next.js website emits `out/third-party-notices.md` through
  `license-checker-rseidelsohn`.

Generated reports support compliance review; they do not replace legal review
of unknown, custom, reciprocal, or distribution-specific license obligations.