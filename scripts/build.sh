#!/bin/sh
set -eu
cd -- "$(dirname -- "$0")/.."

bun install --cwd web --frozen-lockfile &&
	bun run --cwd web build &&
	go build -tags embed -o bin/oib ./cmd/server
