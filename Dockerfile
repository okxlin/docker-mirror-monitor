FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine3.24@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS builder
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=dev
ARG BUILD_TIME=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" -o /out/docker-mirror-monitor ./main.go

FROM alpine:3.24@sha256:71e9b7bae160ba167fa17414401b94cbcdaf3c32feeab9298bd794a2a0614c35
ENV TZ=Asia/Shanghai

# Refresh packages already installed in the pinned base image.
RUN apk --no-cache upgrade && \
    apk --no-cache add ca-certificates tzdata && \
    addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup && \
    mkdir -p /app/data && \
    chown 1000:1000 /app/data && \
    chmod 755 /app/data

WORKDIR /app
COPY --from=builder --chown=1000:1000 --chmod=755 /out/docker-mirror-monitor /app/docker-mirror-monitor
COPY --chown=1000:1000 --chmod=644 config.yaml /app/data/config.yaml

USER 1000:1000
EXPOSE 9080
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD ["/app/docker-mirror-monitor", "healthcheck", "-config", "/app/data/config.yaml"]
ENTRYPOINT ["/app/docker-mirror-monitor"]
CMD ["-config", "/app/data/config.yaml"]
