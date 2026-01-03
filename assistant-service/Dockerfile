# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git for fetching dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o assistant-service ./cmd/assistant-service

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/assistant-service .

# Create prompts directory
RUN mkdir -p /app/prompts

EXPOSE 8096

CMD ["./assistant-service"]
