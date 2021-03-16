FROM quay.io/prometheus/golang-builder:1.14-main as builder

ADD .   /go/src/github.com/justwatchcom/elasticsearch_exporter
WORKDIR /go/src/github.com/justwatchcom/elasticsearch_exporter

RUN make

FROM quay.io/prometheus/busybox:latest
MAINTAINER  The Prometheus Authors <prometheus-developers@googlegroups.com>

COPY --from=builder /go/src/github.com/justwatchcom/elasticsearch_exporter/elasticsearch_exporter  /bin/elasticsearch_exporter

EXPOSE      9114
ENTRYPOINT  [ "/bin/elasticsearch_exporter" ]
