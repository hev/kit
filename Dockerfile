# The kit dashboard: `hev serve`, as `hev up` runs it beside the gateway.
#
#   docker build --build-arg VERSION=0.1.0 -t hevlayer/kit:0.1.0 .
#
# The image carries the whole `hev` binary, but only `serve` is meant to run
# here: capture reads transcripts off the host and runs there under launchd.
FROM --platform=$BUILDPLATFORM golang:1.24 AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY skills ./skills
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X github.com/hev/kit/internal/version.Version=${VERSION}" \
      -o /out/hev ./cmd/hev

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hev /usr/local/bin/hev
EXPOSE 8099
ENTRYPOINT ["/usr/local/bin/hev"]
CMD ["serve", "--bind", "0.0.0.0", "--port", "8099"]
