FROM node:18-alpine AS builder
WORKDIR /app
COPY Frontend/package.json Frontend/package-lock.json ./Frontend/
WORKDIR /app/Frontend
RUN npm install
COPY Frontend .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/Frontend/dist /usr/share/nginx/html
COPY Frontend/public /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
