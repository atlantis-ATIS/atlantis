FROM alpine:3.7

RUN \
  apk add --update go git make gcc musl-dev linux-headers ca-certificates && \
  git clone --depth 1 --branch release/1.8 https://github.com/athereum/go-athereum && \
  (cd go-athereum && make gath) && \
  cp go-athereum/build/bin/gath /gath && \
  apk del go git make gcc musl-dev linux-headers && \
  rm -rf /go-athereum && rm -rf /var/cache/apk/*

EXPOSE 8545
EXPOSE 30303

ENTRYPOINT ["/gath"]
