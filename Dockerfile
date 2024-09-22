FROM alpine:3

RUN apk add --no-cache tini \
    curl \
    git \
    jq \
    bash \
    make \
    openssh-client

COPY directory-watcher /

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/sbin/tini", "--", "/entrypoint.sh"]
CMD ["-h"]
