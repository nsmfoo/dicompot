FROM alpine:latest
RUN mkdir -p /opt/go/
ENV GOPATH=/opt/go/
RUN apk -U add go build-base g++
COPY . /opt/go/dicompot
RUN cd /opt/go/dicompot && go install server/dicompot.go
CMD /opt/go/bin/dicompot