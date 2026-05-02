# syntax=docker/dockerfile:1

FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY matter-commissioner ./matter-commissioner
WORKDIR /src/matter-commissioner
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/matter-commissioner ./cmd/matter-commissioner
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/chip-tool-wrapper ./cmd/chip-tool-wrapper

FROM ghcr.io/project-chip/chip-build:latest AS chip-tool-builder
ARG TARGETPLATFORM
ARG CHIP_SDK_REF=master
SHELL ["/bin/bash", "-lc"]
WORKDIR /opt
RUN git clone --depth=1 --branch "${CHIP_SDK_REF}" https://github.com/project-chip/connectedhomeip.git
WORKDIR /opt/connectedhomeip
RUN ./scripts/checkout_submodules.py --allow-changing-global-git-config --shallow --platform linux
RUN bash ./scripts/bootstrap.sh
RUN set -eo pipefail \
		&& source scripts/activate.sh \
		&& case "${TARGETPLATFORM}" in \
				"linux/amd64") export CHIP_TOOL_TARGET="linux-x64-chip-tool-ipv6only-platform-mdns-nfc-commission" ;; \
				"linux/arm64") export CHIP_TOOL_TARGET="linux-arm64-chip-tool-ipv6only-platform-mdns-nfc-commission" ;; \
				*) echo "Unsupported TARGETPLATFORM: ${TARGETPLATFORM}" >&2; exit 1 ;; \
			esac \
		&& scripts/build/build_examples.py --target "${CHIP_TOOL_TARGET}" build \
		&& cp "out/${CHIP_TOOL_TARGET}/chip-tool" /out-chip-tool

FROM ghcr.io/project-chip/chip-cirque-device-base:latest
WORKDIR /app
COPY --from=builder /out/matter-commissioner /app/matter-commissioner
COPY --from=builder /out/chip-tool-wrapper /app/bin/chip-tool-wrapper
COPY matter-commissioner/scripts/entrypoint.sh /app/bin/entrypoint.sh
COPY --from=chip-tool-builder /out-chip-tool /usr/bin/chip-tool
RUN mkdir -p /var/lib/matter-commissioner/chip-tool /run/dbus /run/avahi-daemon \
	&& chmod +x /app/bin/entrypoint.sh
EXPOSE 8098
ENTRYPOINT ["/app/bin/entrypoint.sh"]