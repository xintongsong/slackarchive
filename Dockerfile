FROM golang:latest AS builder

ADD . /go/src/github.com/ashb/slackarchive
WORKDIR /go/src/github.com/ashb/slackarchive

ARG LDFLAGS=""
RUN go build -tags="" -o /go/bin/app github.com/ashb/slackarchive

FROM debian
RUN apt-get update && apt-get install -y ca-certificates
COPY --from=builder /go/bin/app /slackarchive/slackarchive

RUN mkdir /config

ENTRYPOINT ["/slackarchive/slackarchive", "--config", "/config/config.yaml"]

