FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY device-hub ./device-hub
WORKDIR /src/device-hub
RUN GOWORK=off go mod download
RUN GOWORK=off go build -o /out/device-hub ./cmd/devicehub

FROM alpine:3.22
WORKDIR /app
COPY --from=builder /out/device-hub /app/device-hub
EXPOSE 8090
ENTRYPOINT ["/app/device-hub"]
