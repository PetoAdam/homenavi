FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY user-service ./user-service
WORKDIR /src/user-service
RUN GOWORK=off go mod download
RUN GOWORK=off go build -o /out/user-service

FROM alpine:3.22
WORKDIR /app
COPY --from=builder /out/user-service ./user-service
EXPOSE 8001
CMD ["./user-service"]
