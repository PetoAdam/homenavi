FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o zigbee-adapter ./cmd/zigbee-adapter

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/zigbee-adapter /app/zigbee-adapter
EXPOSE 8091
ENTRYPOINT ["/app/zigbee-adapter"]
