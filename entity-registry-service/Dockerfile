FROM golang:1.24-alpine AS builder
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o entity-registry-service ./cmd/entity-registry-service

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/entity-registry-service /app/entity-registry-service
EXPOSE 8095
ENTRYPOINT ["/app/entity-registry-service"]
