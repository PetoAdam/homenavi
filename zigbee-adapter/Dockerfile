FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY zigbee-adapter ./zigbee-adapter
WORKDIR /src/zigbee-adapter
RUN GOWORK=off go mod download
RUN GOWORK=off go build -o /out/zigbee-adapter ./cmd/zigbee-adapter

FROM alpine:3.22
WORKDIR /app
COPY --from=builder /out/zigbee-adapter /app/zigbee-adapter
EXPOSE 8091
ENTRYPOINT ["/app/zigbee-adapter"]
