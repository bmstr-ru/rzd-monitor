#!/bin/bash

GOOS=linux GOARCH=amd64 go build .
docker build . -t rzd-monitor:latest