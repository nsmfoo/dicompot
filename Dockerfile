FROM alpine:latest
RUN apk -U add go build-base g++
RUN mkdir -p /opt/go
RUN export GOPATH=/opt/go/
COPY . /opt/go/dicompot
RUN cd /opt/go/dicompot
RUN go mod download
RUN go install -a -x github.com/nsmfoo/dicompot/server
CMD /opt/go/bin/dicompot
