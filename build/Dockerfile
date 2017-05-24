FROM golang:latest

RUN mkdir -p /app
COPY . /app/
WORKDIR /app

ENV CGO_ENABLED 0
ENV GOOS linux

RUN go get -d ./...
RUN go build -o webUIcompare

ENTRYPOINT ["/bin/sh"]
