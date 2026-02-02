FROM --platform=$BUILDPLATFORM golang:1.24.12-alpine AS builder
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG APP_NAME=docker-mirror-monitor

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o ${APP_NAME} main.go

FROM alpine:3.18
ENV TZ=Asia/Shanghai APP_NAME=docker-mirror-monitor

RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

WORKDIR /app

RUN mkdir -p /app/data && \
    chown -R 1000:1000 /app/data && \
    chmod 755 /app/data

COPY --from=builder --chown=1000:1000 --chmod=755 /app/${APP_NAME} /app/${APP_NAME}
COPY --chown=1000:1000 --chmod=644 config.yaml* /app/data/

EXPOSE 9080
USER 1000

CMD ["/app/docker-mirror-monitor", "-config", "/app/data/config.yaml"]
