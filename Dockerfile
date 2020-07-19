FROM alpine:latest
RUN mkdir -p /opt/go/
ENV GOPATH=/opt/go/
RUN apk -U add go build-base g++
COPY . /opt/go/dicompot
RUN cd /opt/go/dicompot && go mod download
RUN cd /opt/go/dicompot && go install -a -x github.com/nsmfoo/dicompot/server
CMD /opt/go/bin/server
