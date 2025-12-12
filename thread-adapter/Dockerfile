FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o thread-adapter ./cmd/thread-adapter

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/thread-adapter /app/thread-adapter
EXPOSE 8092
ENTRYPOINT ["/app/thread-adapter"]
