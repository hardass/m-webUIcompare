#!/bin/sh

docker rm -f webuicompare
docker rmi -f webuicompare

docker build --tag webuicompare build
docker run --name webuicompare webuicompare

rm -rf ship/webUIcompare
docker cp webuicompare:/app/webUIcompare ship/

docker rm -f webuicompare
docker build --tag webuicompare ship

