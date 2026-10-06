# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY store ./store
COPY httpapi ./httpapi
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/marchiobj ./cmd/marchiobj

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl \
	&& adduser -D -u 10001 marchi \
	&& mkdir -p /data \
	&& chown marchi:marchi /data
COPY --from=build /out/marchiobj /usr/local/bin/marchiobj
USER marchi
WORKDIR /data
VOLUME ["/data"]
EXPOSE 7300
ENV MARCHIOBJ_ADDR=:7300 \
	MARCHIOBJ_DATA=/data
HEALTHCHECK --interval=10s --timeout=3s --start-period=3s --retries=3 \
	CMD curl -fsS http://127.0.0.1:7300/healthz || exit 1
ENTRYPOINT ["marchiobj"]
CMD ["-addr=:7300", "-dataDir=/data"]
