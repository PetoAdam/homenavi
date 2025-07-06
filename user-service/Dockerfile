FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o user-service

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/user-service .
EXPOSE 8001
CMD ["./user-service"]
