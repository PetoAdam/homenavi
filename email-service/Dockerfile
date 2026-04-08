FROM golang:1.26.1-alpine3.22 AS builder

WORKDIR /src
COPY shared ./shared
COPY email-service ./email-service
WORKDIR /src/email-service
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -a -installsuffix cgo -o /out/main .

# Final stage
FROM alpine:3.22

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /out/main .

# Expose port
EXPOSE 8002

# Command to run
CMD ["./main"]