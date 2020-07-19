FROM alpine:latest
RUN apk -U add go build-base g++ git
RUN mkdir -p /opt/go
RUN export GOPATH=/opt/go/
RUN cd /opt/go/
RUN git clone https://github.com/nsmfoo/dicompot.git
RUN cd dicompot
RUN go mod download
RUN go install -a -x github.com/nsmfoo/dicompot/server
CMD /opt/go/bin/dicompot
