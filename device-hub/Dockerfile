FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o device-hub ./cmd/devicehub

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/device-hub /app/device-hub
EXPOSE 8090
ENTRYPOINT ["/app/device-hub"]
