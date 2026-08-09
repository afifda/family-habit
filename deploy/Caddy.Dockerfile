FROM golang:1.25.12-alpine3.24 AS build
WORKDIR /src
RUN go mod init family-habit-caddy-build \
    && go get github.com/caddyserver/caddy/v2/cmd/caddy@v2.11.4 \
    && go mod edit -replace=golang.org/x/text=golang.org/x/text@v0.39.0 \
    && go mod edit -replace=google.golang.org/grpc=google.golang.org/grpc@v1.82.1 \
    && go mod download all \
    && CGO_ENABLED=0 go build -o /caddy github.com/caddyserver/caddy/v2/cmd/caddy

FROM alpine:3.24.1
RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates libcap mailcap \
    && addgroup -S caddy \
    && adduser -S -D -H -G caddy caddy \
    && mkdir -p /data /config \
    && chown -R caddy:caddy /data /config
COPY --from=build /caddy /usr/bin/caddy
RUN setcap cap_net_bind_service=+ep /usr/bin/caddy
COPY Caddyfile /etc/caddy/Caddyfile
EXPOSE 80 443 443/udp
USER caddy
ENTRYPOINT ["caddy"]
CMD ["run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]
