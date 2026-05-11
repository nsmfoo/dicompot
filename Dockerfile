FROM golang:1.26-alpine
WORKDIR /opt/dicompot/
COPY go.mod go.sum ./
ENV GOCACHE=/go-cache
ENV GOMODCACHE=/gomod-cache
COPY . ./
RUN go install server/dicompot.go
CMD ["/go/bin/dicompot", "-ip", "0.0.0.0"]