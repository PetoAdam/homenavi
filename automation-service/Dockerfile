FROM golang:1.24-alpine AS builder
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o automation-service ./cmd/automation-service

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/automation-service /app/automation-service
EXPOSE 8094
ENTRYPOINT ["/app/automation-service"]
