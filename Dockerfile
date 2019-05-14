# Build the manager binary
FROM golang:1.10.3 as builder

RUN curl -fsSL -o /usr/local/bin/dep https://github.com/golang/dep/releases/download/v0.5.3/dep-linux-amd64 \
 && chmod +x /usr/local/bin/dep

# Copy in the go src
WORKDIR /go/src/github.com/bgpat/job-slack-notifier

COPY Gopkg.toml Gopkg.toml
COPY Gopkg.lock Gopkg.lock
RUN dep ensure -vendor-only

COPY pkg/    pkg/
COPY cmd/    cmd/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager github.com/bgpat/job-slack-notifier/cmd/manager

# Copy the controller-manager into a thin image
FROM debian:buster-slim
RUN apt-get update && apt-get install -y ca-certificates \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /
COPY --from=builder /go/src/github.com/bgpat/job-slack-notifier/manager .
ENTRYPOINT ["/manager"]
