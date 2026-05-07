FROM node:24.14.1 AS build
WORKDIR /app
ARG HOMENAVI_APP_VERSION=dev
ARG GITHUB_SHA=
ARG VITE_INTEGRATION_OPERATION_TIMEOUT_MS=600000
ENV HOMENAVI_APP_VERSION=${HOMENAVI_APP_VERSION}
ENV GITHUB_SHA=${GITHUB_SHA}
ENV VITE_INTEGRATION_OPERATION_TIMEOUT_MS=${VITE_INTEGRATION_OPERATION_TIMEOUT_MS}
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . ./
RUN npm run build

FROM golang:1.26.1-alpine3.22 AS server-build
WORKDIR /src
COPY proxy/go.mod ./
COPY proxy/main.go ./
RUN CGO_ENABLED=0 GOWORK=off go build -o /out/frontend-proxy .

FROM scratch
COPY --from=server-build /out/frontend-proxy /frontend-proxy
COPY --from=build /app/dist /app/dist
EXPOSE 80
ENTRYPOINT ["/frontend-proxy"]