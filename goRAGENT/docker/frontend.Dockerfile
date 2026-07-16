# goRAGENT 前端 Dockerfile — 多阶段构建
# 和 CarAgent 风格一致

# ============================================================
# Stage 1: Build
# ============================================================
FROM node:20-alpine AS build

WORKDIR /build

ARG VITE_API_BASE=/api/ragent
ENV VITE_API_BASE=/api/ragent

# 复制前端依赖
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

# 复制源码并构建
COPY frontend/ ./
RUN npm run build

# ============================================================
# Stage 2: Nginx Runtime
# ============================================================
FROM nginx:alpine

COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /build/dist /usr/share/nginx/html

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
