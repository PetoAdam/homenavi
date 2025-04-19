# syntax=docker/dockerfile:1
FROM node:18-alpine AS builder
WORKDIR /app
COPY homenavi/package.json homenavi/package-lock.json ./homenavi/
WORKDIR /app/homenavi
RUN npm install
COPY homenavi .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/homenavi/dist /usr/share/nginx/html
COPY homenavi/public /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
