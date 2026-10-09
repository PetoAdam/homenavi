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

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80