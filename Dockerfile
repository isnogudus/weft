# weft -- multi-stage build: Svelte SPA -> static Go binary -> minimal runtime.
#
# The SPA is embedded into the binary via embed.FS, so the runtime stage only
# needs the binary, CA certificates (for ldaps://) and the _weft user that the
# privilege-separated worker drops to when the container runs as root.

# --- 1. frontend -------------------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- 2. backend --------------------------------------------------------------
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/weft ./cmd/weft

# --- 3. runtime --------------------------------------------------------------
FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
    && addgroup -S _weft \
    && adduser -S -D -H -G _weft _weft \
    && mkdir -p /var/empty

COPY --from=build /out/weft /usr/local/bin/weft

# Inside a container weft must bind on all interfaces (the default is
# 127.0.0.1:8080). Env beats the config file, so override here.
ENV WEFT_LISTEN_ADDR=0.0.0.0:8080
EXPOSE 8080

# Started as root (the default), weft runs its privsep model: the monitor keeps
# the LDAP dialing, the HTTP worker chroots to /var/empty and drops to _weft.
# Run the container with a non-root user to skip chroot/privdrop entirely.
ENTRYPOINT ["/usr/local/bin/weft"]
# Configuration comes from WEFT_* environment variables by default; mount a
# TOML file and append "-config /etc/weft/weft.toml" for the full option set
# (e.g. [[user_attr]] tables, which have no env equivalent).
CMD []
