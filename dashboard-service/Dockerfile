FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/dashboard-service ./cmd/dashboard-service

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/dashboard-service /app/dashboard-service
EXPOSE 8097
ENTRYPOINT ["/app/dashboard-service"]
