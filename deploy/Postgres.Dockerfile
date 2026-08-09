FROM postgres:16.14-alpine3.24
RUN apk upgrade --no-cache && rm -f /usr/local/bin/gosu
USER postgres
