# Build stage — use pnpm from repo root (build context must be repo root)
FROM node:25-alpine AS builder

# Install pnpm (node:25-alpine does not include corepack; use npm)
RUN npm install -g pnpm@9

WORKDIR /app

# Copy workspace and frontend package files (when context is repo root: docker build -f frontend/Dockerfile .)
COPY pnpm-lock.yaml pnpm-workspace.yaml package.json ./
COPY frontend/package.json frontend/package.json

# Install dependencies (workspace root)
RUN pnpm install --frozen-lockfile

# Copy frontend source
COPY frontend frontend

WORKDIR /app/frontend

# Build
ARG VITE_API_URL
ENV VITE_API_URL=$VITE_API_URL
RUN pnpm run build

# Production stage
FROM nginx:1.29.5-alpine

# Copy built assets
COPY --from=builder /app/frontend/dist /usr/share/nginx/html

# Copy nginx config
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf

# Expose port
EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
