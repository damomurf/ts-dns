ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest

COPY ts-dns /usr/bin/ts-dns

ENTRYPOINT ["/usr/bin/ts-dns"]
