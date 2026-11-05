#!/bin/bash
set -e
cd "$(dirname "$0")"
mkdir -p out
GOOS=linux GOARCH=amd64 go build -o out/ahsanly-api ./cmd/api
GOOS=linux GOARCH=amd64 go build -o out/ahsanly-worker ./cmd/worker
GOOS=linux GOARCH=amd64 go build -o out/ahsanly-pipeline ./cmd/pipeline
GOOS=linux GOARCH=amd64 go build -o out/ahsanly-publish ./cmd/publish
GOOS=linux GOARCH=amd64 go build -o out/ahsanly-discover ./cmd/discover
GOOS=linux GOARCH=amd64 go build -o out/ahsanly-migrate ./cmd/migrate
GOOS=linux GOARCH=amd64 go build -o out/ahsanly-testphoto ./cmd/testphoto
GOOS=linux GOARCH=amd64 go build -o out/ahsanly-capture ./cmd/capture
GOOS=linux GOARCH=amd64 go build -o out/ahsanly-probe ./cmd/probe
GOOS=linux GOARCH=amd64 go build -o out/ahsanly-geoipverify ./cmd/geoipverify
echo "Linux binaries built in out/"
