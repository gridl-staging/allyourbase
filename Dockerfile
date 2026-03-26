FROM node:20-alpine AS ui-builder

WORKDIR /src/ui
COPY ui/package.json ui/pnpm-lock.yaml ./
RUN npm install --legacy-peer-deps
COPY ui/ .
RUN npm run build

FROM node:20-alpine AS demo-builder

WORKDIR /src/examples/kanban
COPY examples/kanban/package*.json ./
RUN npm ci
COPY examples/kanban/ .
RUN VITE_AYB_URL="" npx vite build

WORKDIR /src/examples/live-polls
COPY examples/live-polls/package*.json ./
RUN npm ci
COPY examples/live-polls/ .
RUN VITE_AYB_URL="" npx vite build

FROM alpine:3.22 AS pg-builder

RUN apk add --no-cache \
    bash \
    bison \
    build-base \
    coreutils \
    curl \
    e2fsprogs-dev \
    flex \
    libxml2-dev \
    linux-headers \
    openssl-dev \
    perl \
    tar \
    xz

WORKDIR /src
COPY scripts/build-postgres.sh ./scripts/build-postgres.sh

RUN set -euo pipefail; \
    arch="$(uname -m)"; \
    case "$arch" in \
        x86_64) ayb_arch="amd64" ;; \
        aarch64) ayb_arch="arm64" ;; \
        *) echo "unsupported arch: $arch" >&2; exit 1 ;; \
    esac; \
    bash ./scripts/build-postgres.sh --pg-version 16 --os linux --arch "$ayb_arch" --output-dir /tmp/pg-dist; \
    mkdir -p /opt/ayb-managed-pg; \
    tar -xJf "/tmp/pg-dist/ayb-postgres-16-linux-${ayb_arch}.tar.xz" -C /opt/ayb-managed-pg --strip-components=1

FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git build-base libwebp-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /src/ui/dist ./ui/dist
COPY --from=demo-builder /src/examples/kanban/dist ./examples/kanban/dist
COPY --from=demo-builder /src/examples/live-polls/dist ./examples/live-polls/dist

RUN go build -ldflags "-s -w" -o /ayb ./cmd/ayb

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata libgcc libstdc++ libwebp libxml2 openssl su-exec
RUN addgroup -S ayb && adduser -S -D -h /home/ayb -G ayb ayb && install -d -o ayb -g ayb /home/ayb/.ayb

COPY --from=pg-builder --chown=ayb:ayb /opt/ayb-managed-pg /home/ayb/.ayb/pgbin
COPY --from=builder /ayb /usr/local/bin/ayb
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENV HOME=/home/ayb
ENV LD_LIBRARY_PATH=/home/ayb/.ayb/pgbin/lib
ENV AYB_SERVER_HOST=0.0.0.0

WORKDIR /home/ayb

EXPOSE 8090

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["ayb", "start", "--foreground"]
