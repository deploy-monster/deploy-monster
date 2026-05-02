# syntax=docker/dockerfile:1.7
#
# Release Dockerfile — consumed by goreleaser `dockers_v2:` on tag push.
# The deploymonster binary (with embedded React SPA) is provided by goreleaser
# in the build context. This Dockerfile only prepares the minimal scratch
# runtime: CA certs, tzdata, and a pre-chowned data directory.
# No shell, no package manager, no curl.
#
# Local dev builds should use deployments/Dockerfile + docker-compose.dev.yaml
# instead — that path is optimized for hot rebuilds, not minimal footprint.

# ─── Stage 1: Prepare CA certs and data directory for scratch ────────────────
FROM alpine:3.21 AS rootfs
RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /rootfs/var/lib/deploymonster \
    && chown -R 65534:65534 /rootfs/var/lib/deploymonster

# ─── Stage 2: Minimal scratch runtime ────────────────────────────────────────
FROM scratch
ARG TARGETPLATFORM

COPY --from=rootfs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=rootfs /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=rootfs /rootfs/ /

# The binary is placed in a platform-specific context path by goreleaser; UI is already embedded.
COPY ${TARGETPLATFORM}/deploymonster /deploymonster

VOLUME ["/var/lib/deploymonster"]
EXPOSE 8443 80 443

USER 65534:65534

LABEL org.opencontainers.image.title="DeployMonster" \
      org.opencontainers.image.description="Self-hosted PaaS — single binary, modular monolith, event-driven" \
      org.opencontainers.image.url="https://deploy.monster" \
      org.opencontainers.image.source="https://github.com/deploy-monster/deploy-monster" \
      org.opencontainers.image.vendor="ECOSTACK TECHNOLOGY OÜ" \
      org.opencontainers.image.licenses="AGPL-3.0"

ENTRYPOINT ["/deploymonster"]
CMD ["serve"]
