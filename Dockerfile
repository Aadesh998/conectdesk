# Builds the frontend and backend from source (used by Railway).
# Dockerfile.release is used by GoReleaser and expects a prebuilt binary.
FROM node:22-alpine AS frontend
RUN corepack enable
WORKDIR /src/frontend
ARG CONECTDESK_VERSION=v0.0.0
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend ./
# The main app imports stylesheets from the repo-root static/ directory.
COPY static /src/static
RUN VITE_APP_VERSION="${CONECTDESK_VERSION}" pnpm build:main \
    && VITE_APP_VERSION="${CONECTDESK_VERSION}" pnpm build:widget

FROM golang:1.25-alpine AS backend
RUN apk --no-cache add git
WORKDIR /src
ARG CONECTDESK_VERSION=v0.0.0
COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/knadh/stuffbin/...@latest
COPY . .
COPY --from=frontend /src/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w \
      -X 'main.versionString=${CONECTDESK_VERSION}' \
      -X 'main.buildString=${CONECTDESK_VERSION} (railway)' \
      -X 'github.com/conectdesk/conectdesk/internal/version.Version=${CONECTDESK_VERSION}'" \
      -o conectdesk ./cmd/
RUN stuffbin -a stuff -in conectdesk -out conectdesk frontend/dist i18n schema static

FROM alpine:3.18
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /conectdesk
COPY --from=backend /src/conectdesk .
COPY config.sample.toml config.toml
EXPOSE 9000
# Railway injects $PORT, and the mount path of the volume that holds uploads.
# Install the schema on first boot and apply migrations on every deploy before
# starting, as docker-compose.yml does, and create the platform admin from
# PLATFORM_ADMIN_EMAIL/PASSWORD when set; a rejected one is logged and the
# server starts anyway. Once uploads are on S3/MinIO, files still on the
# volume are copied there in the background, once.
CMD ["sh", "-c", "export CONECTDESK_app__server__address=\"0.0.0.0:${PORT:-9000}\" CONECTDESK_upload__fs__upload_path=\"${RAILWAY_VOLUME_MOUNT_PATH:-uploads}\" && ./conectdesk --install --idempotent-install --yes && ./conectdesk --upgrade --yes && if [ \"$CONECTDESK_upload__provider\" = s3 ] && [ -n \"$RAILWAY_VOLUME_MOUNT_PATH\" ] && [ ! -f \"$RAILWAY_VOLUME_MOUNT_PATH/.copied-to-s3\" ]; then (./conectdesk --copy-uploads-to-s3 && touch \"$RAILWAY_VOLUME_MOUNT_PATH/.copied-to-s3\") & fi && if [ -n \"$PLATFORM_ADMIN_EMAIL\" ]; then echo \"$PLATFORM_ADMIN_PASSWORD\" | ./conectdesk --platform-admin-add \"$PLATFORM_ADMIN_EMAIL\" || echo \"platform admin not created, fix PLATFORM_ADMIN_EMAIL/PASSWORD; starting anyway\"; fi && exec ./conectdesk"]
