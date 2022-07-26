FROM golang:latest AS builder
ADD . /go/src/github.com/ashb/slackarchive
WORKDIR /go/src/github.com/ashb/slackarchive
RUN go build -trimpath -o ./slackarchive ./main.go

FROM debian
RUN apt-get update && apt-get install -y ca-certificates
ADD ./wait-for-it.sh /slackarchive/wait-for-it.sh
COPY --from=builder /go/src/github.com/ashb/slackarchive/slackarchive /slackarchive/slackarchive
WORKDIR /slackarchive