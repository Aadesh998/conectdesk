FROM node:22-alpine AS frontend
RUN corepack enable
WORKDIR /src/frontend
ARG CONECTDESK_VERSION=v0.0.0
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN --mount=type=cache,id=s/a7645c50-c188-43a8-b349-c2743e517195-/pnpm/store,target=/pnpm/store \
    pnpm install --frozen-lockfile --store-dir /pnpm/store
COPY frontend ./
# The main app imports stylesheets and icons from the repo-root static/public/static.
COPY static/public/static /src/static/public/static
RUN VITE_APP_VERSION="${CONECTDESK_VERSION}" pnpm build:main \
    && VITE_APP_VERSION="${CONECTDESK_VERSION}" pnpm build:widget

FROM golang:1.25-alpine AS backend
RUN apk --no-cache add git
WORKDIR /src
ARG CONECTDESK_VERSION=v0.0.0
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
# stuffbin at the version go.mod pins, rather than @latest.
RUN --mount=type=cache,id=s/a7645c50-c188-43a8-b349-c2743e517195-/go/pkg/mod,target=/go/pkg/mod \
    --mount=type=cache,id=s/a7645c50-c188-43a8-b349-c2743e517195-/root/.cache/go-build,target=/root/.cache/go-build \
    go mod download && go install github.com/knadh/stuffbin/stuffbin
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,id=s/a7645c50-c188-43a8-b349-c2743e517195-/go/pkg/mod,target=/go/pkg/mod \
    --mount=type=cache,id=s/a7645c50-c188-43a8-b349-c2743e517195-/root/.cache/go-build,target=/root/.cache/go-build \
    go build -ldflags="-s -w \
      -X 'main.versionString=${CONECTDESK_VERSION}' \
      -X 'main.buildString=${CONECTDESK_VERSION} (railway)' \
      -X 'github.com/conectdesk/conectdesk/internal/version.Version=${CONECTDESK_VERSION}'" \
      -o conectdesk ./cmd/
# The files bundled into the binary.
COPY i18n ./i18n
COPY schema ./schema
COPY static ./static
COPY --from=frontend /src/frontend/dist ./frontend/dist
RUN stuffbin -a stuff -in conectdesk -out conectdesk frontend/dist i18n schema static

FROM alpine:3.18
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /conectdesk
COPY --from=backend /src/conectdesk .
COPY config.sample.toml config.toml
EXPOSE 9000

CMD ["sh", "-c", "export CONECTDESK_app__server__address=\"0.0.0.0:${PORT:-9000}\" CONECTDESK_upload__fs__upload_path=\"${RAILWAY_VOLUME_MOUNT_PATH:-uploads}\" && ./conectdesk --install --idempotent-install --yes && ./conectdesk --upgrade --yes && if [ \"$CONECTDESK_upload__provider\" = s3 ] && [ -n \"$RAILWAY_VOLUME_MOUNT_PATH\" ] && [ ! -f \"$RAILWAY_VOLUME_MOUNT_PATH/.copied-to-s3\" ]; then (./conectdesk --copy-uploads-to-s3 && touch \"$RAILWAY_VOLUME_MOUNT_PATH/.copied-to-s3\") & fi && if [ -n \"$PLATFORM_ADMIN_EMAIL\" ]; then echo \"$PLATFORM_ADMIN_PASSWORD\" | ./conectdesk --platform-admin-add \"$PLATFORM_ADMIN_EMAIL\" || echo \"platform admin not created, fix PLATFORM_ADMIN_EMAIL/PASSWORD; starting anyway\"; fi && if [ -n \"$DOCS_VIEWER_EMAIL\" ]; then echo \"$DOCS_VIEWER_PASSWORD\" | ./conectdesk --docs-viewer-add \"$DOCS_VIEWER_EMAIL\" || echo \"docs viewer not created, fix DOCS_VIEWER_EMAIL/PASSWORD; starting anyway\"; fi && exec ./conectdesk"]
