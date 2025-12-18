FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod .
RUN go mod download
COPY . .
RUN go build -o history-service ./cmd/history-service

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/history-service /app/history-service
EXPOSE 8093
ENTRYPOINT ["/app/history-service"]
