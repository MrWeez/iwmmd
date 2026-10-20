# syntax=docker/dockerfile:1.7

FROM node:24-alpine AS web

RUN npm install -g pnpm@10.30.3
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.27-alpine AS build

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
RUN mkdir -p /out/data && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w \
        -X github.com/mrweez/iwmmd/internal/platform/version.Version=${VERSION} \
        -X github.com/mrweez/iwmmd/internal/platform/version.Commit=${COMMIT} \
        -X github.com/mrweez/iwmmd/internal/platform/version.Date=${BUILD_DATE}" \
      -o /out/iwmmd ./cmd/iwmmd

FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="IWMMD" \
      org.opencontainers.image.description="Self-hosted domain expiration monitor" \
      org.opencontainers.image.source="https://github.com/mrweez/iwmmd" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

ENV IWMMD_DATA_DIR=/data \
    IWMMD_LOG_FORMAT=json

COPY --from=build --chown=nonroot:nonroot /out/iwmmd /iwmmd
COPY --from=build --chown=nonroot:nonroot /out/data /data

USER nonroot:nonroot
WORKDIR /
VOLUME ["/data"]
EXPOSE 8080 8443

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/iwmmd", "healthcheck", "--url", "http://127.0.0.1:8080/healthz"]

ENTRYPOINT ["/iwmmd"]
CMD ["serve"]
