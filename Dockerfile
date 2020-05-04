ARG FROM=alpine
FROM $FROM
ARG GOARCH
LABEL maintainer="squat <lserven@gmail.com>"
COPY bin/$GOARCH/kilo-wg-gen-web /usr/local/bin/kilo-wg-gen-web
ENTRYPOINT ["/usr/local/bin/kilo-wg-gen-web"]
