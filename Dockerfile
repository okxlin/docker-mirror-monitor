FROM --platform=$BUILDPLATFORM golang:1.26.6-alpine3.24@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags="-s -w" -o /out/docker-mirror-monitor ./main.go

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
ENV TZ=Asia/Shanghai

RUN apk --no-cache add ca-certificates tzdata && \
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
