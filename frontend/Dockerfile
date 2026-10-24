# Build stage: use pnpm from repo root (build context must be repo root).
# BUILDPLATFORM keeps the Node build native; the nginx stage stays per-arch.
# Images are pinned by digest; Dependabot and Renovate move the digest when the tag moves.
FROM --platform=$BUILDPLATFORM node:26-alpine@sha256:40cd011b55415bc894e45b02fdd5a5dfde056921b7d23a64c74f80497af48797 AS builder

WORKDIR /app

# Install the pnpm version declared in package.json. A global pnpm 11 install
# tries to launch a newer packageManager build as a Node module and crashes
# when that build is a native binary.
COPY package.json ./
RUN npm install -g "$(node -e 'process.stdout.write(require("./package.json").packageManager)')"

# Copy workspace and frontend package files (when context is repo root: docker build -f frontend/Dockerfile .)
COPY pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/package.json frontend/package.json

# Install dependencies (workspace root)
RUN pnpm install --frozen-lockfile

# Copy frontend source
COPY frontend frontend

WORKDIR /app/frontend

# Build. VITE_API_URL stays empty: the API is served same-origin through nginx.
ARG VITE_API_URL=
ENV VITE_API_URL=$VITE_API_URL
RUN pnpm run build

# Production stage: unprivileged nginx (listens on 8080, runs as uid 101, writes only under /tmp)
FROM nginxinc/nginx-unprivileged:stable-alpine@sha256:15c994d10d6d78658721c3bcafff14cb281fba2a4bdf9d5ba92c416a472516e3

USER root
# Pull every package fix published since the base image was built
RUN apk --no-cache upgrade \
    && mkdir -p /etc/nginx/snippets \
    && rm -f /etc/nginx/conf.d/default.conf
USER 101

# Built assets
COPY --from=builder /app/frontend/dist /usr/share/nginx/html

# nginx config: the entrypoint renders /etc/nginx/templates/*.template into /etc/nginx/conf.d
COPY frontend/nginx/security-headers.conf /etc/nginx/snippets/security-headers.conf
COPY frontend/nginx/default.conf.template /etc/nginx/templates/default.conf.template

# Backend address used by the /api and /ws proxy locations
ENV BACKEND_UPSTREAM=http://backend:8080

EXPOSE 8080

CMD ["nginx", "-g", "daemon off;"]
