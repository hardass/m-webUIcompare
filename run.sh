#!/bin/sh

docker rm -f webuicompare

docker run --restart unless-stopped --detach \
  --name webuicompare \
  --publish 8080:8080 \
  webuicompare
