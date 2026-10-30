# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:24-bookworm-slim AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/web/dist ./web/dist
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags production \
    -ldflags "-s -w -X main.version=$VERSION" -o /out/opnsense-ip-invites ./cmd/opnsense-ip-invites
RUN mkdir -p /runtime-data && chmod 0700 /runtime-data

FROM backend AS test
RUN CGO_ENABLED=1 go test -race ./...

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=backend /out/opnsense-ip-invites /app/opnsense-ip-invites
COPY --from=backend --chown=65532:65532 /runtime-data/ /data/
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/opnsense-ip-invites"]
CMD ["-config", "/etc/ip-invites/config.yaml"]
