#!/bin/sh
set -eu
cd -- "$(dirname -- "$0")/.."

pnpm -C web install --frozen-lockfile &&
	pnpm -C web build &&
	go build -tags embed -o bin/oib ./cmd/server
