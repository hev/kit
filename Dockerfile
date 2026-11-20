# The kit dashboard: `hev serve`, as `hev up` runs it beside the gateway.
#
#   docker build --build-arg VERSION=0.1.0 -t hevlayer/kit:0.1.0 .
#
# The image carries the whole `hev` binary. Two commands are meant to run
# here: `serve` (the default CMD) and `collect`, app capture for a host app
# (docs/app-traces.md). Coding-agent capture reads transcripts off the host and
# runs there under launchd or systemd, never in this image.
#
# The `collect` variant: the service key and Layer target come from the
# environment; /data keeps the redaction salt and the trace envelopes. On v0.7
# the target is hosted hev layer (the default); for your own Turbopuffer
# account also pass HEVLAYER_ENDPOINT=<its gateway> and HEVLAYER_STORE=turbopuffer.
# With no HEVLAYER_API_KEY, collect uses the file archive instead: pass
# HEV_ARCHIVE_DIR=/data/archive so turns, boards and envelopes stay on the
# volume (docs/app-traces.md). There is no local Postgres store on v0.7.
#
#   docker run -d --name hev-collect -p 127.0.0.1:4318:4318 -v hev-collect:/data \
#     -e HEV_CONFIG=/data/config.toml -e HEV_COLLECT_ENVELOPES_DIR=/data/envelopes \
#     -e HEV_COLLECT_SERVICE_KEY="<key>" -e HEV_COLLECT_NAMESPACES=story-prod \
#     -e HEVLAYER_API_KEY="<hev layer key>" \
#     hevlayer/kit:0.1.0 collect --listen 0.0.0.0:4318
FROM --platform=$BUILDPLATFORM golang:1.27 AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY prices.go prices.toml ./
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
COPY skills ./skills
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X github.com/hev/kit/internal/version.Version=${VERSION}" \
      -o /out/hev ./cmd/hev
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hev /usr/local/bin/hev
COPY --from=build --chown=nonroot:nonroot /out/data /data
EXPOSE 8099 4318
ENTRYPOINT ["/usr/local/bin/hev"]
CMD ["serve", "--bind", "0.0.0.0", "--port", "8099"]
