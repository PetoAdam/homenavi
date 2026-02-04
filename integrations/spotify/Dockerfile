FROM node:20-alpine AS webbuild
WORKDIR /src

COPY src/frontend/package.json ./src/frontend/package.json
RUN cd src/frontend && npm install

COPY src/frontend ./src/frontend
RUN cd src/frontend && npm run build


FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./
COPY --from=webbuild /src/web/ui ./web/ui
COPY --from=webbuild /src/web/widgets ./web/widgets

RUN CGO_ENABLED=0 go build -o /out/integration ./src/backend/cmd/integration


FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
RUN mkdir -p /app/config

COPY --from=build /out/integration /app/integration
COPY --from=build /src/manifest /app/manifest
COPY --from=build /src/web /app/web

EXPOSE 8099
ENV PORT=8099
ENTRYPOINT ["/app/integration"]
