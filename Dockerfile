# Build a static binary; modernc.org/sqlite needs no cgo.
FROM golang:1.26.10-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bookmarkbox ./cmd/web

# The sqlite package provides the backup command, and BusyBox wget the health check.
FROM alpine:3.24.4
RUN apk add --no-cache sqlite \
    && addgroup -S -g 10001 bookmarkbox \
    && adduser -S -D -H -u 10001 -G bookmarkbox bookmarkbox \
    && mkdir /data \
    && chown bookmarkbox:bookmarkbox /data
COPY --from=build /bookmarkbox /usr/local/bin/bookmarkbox
ENV PORT=4000 DATABASE_PATH=/data/bookmarkbox.db
USER 10001:10001
EXPOSE 4000
HEALTHCHECK --interval=10s --timeout=3s \
    CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/ping"
ENTRYPOINT ["bookmarkbox"]
