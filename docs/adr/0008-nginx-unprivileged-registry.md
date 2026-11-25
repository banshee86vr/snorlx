# 0008: Move the frontend base image to nginx/nginx-unprivileged

- Status: Accepted
- Date: 2026-11-25

## Context

The frontend production stage used `nginxinc/nginx-unprivileged:stable-alpine` pinned by digest. From NGINX 1.30.6 (stable) and 1.31.7 (mainline), the project publishes that image under `nginx/nginx-unprivileged` instead. The old `nginxinc` registry still serves `stable-alpine` at 1.30.6 and has no 1.30.7 tag. Trivy fails the frontend image scan on CRITICAL/HIGH findings fixed only in nginx 1.30.7 (Alpine package `1.30.7-r0`). A full `apk upgrade` in the Dockerfile cannot pull that fix from the stale `nginxinc` image.

## Decision

Pin the frontend production stage to `nginx/nginx-unprivileged:stable-alpine` by digest. Keep the existing `apk upgrade` hardening and the same unprivileged listen port (8080). Let Renovate move the digest when the tag moves.

## Alternatives considered

- Stay on `nginxinc/nginx-unprivileged` and hope for a backported digest. Rejected: the upstream README states fixed 1.30.7 and 1.31.9 tags exist only under `nginx/nginx-unprivileged`.
- Pin a version tag such as `1.30.7-alpine` instead of `stable-alpine`. Rejected for this change to keep the existing floating-tag-plus-digest convention used by the other base images; Renovate already moves digests.
- Replace nginx with another static file server. Rejected: out of scope for clearing the CVE, and ADR 0005 already depends on nginx template proxy behaviour.

## Consequences

Frontend builds pull from Docker Hub `nginx/nginx-unprivileged`. Older docs that still say `nginxinc` describe the pre-migration image. Digests older than 1.30.7 on the new registry must not be used. The same-origin proxy design in ADR 0005 is unchanged.
