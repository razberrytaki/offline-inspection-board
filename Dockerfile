FROM node:24-bookworm-slim AS frontend
WORKDIR /src/web
RUN npm install --global pnpm@12.23.0
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.27.3-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/assets_embed.go ./web/
COPY --from=frontend /src/web/build ./web/build
RUN CGO_ENABLED=0 go build -tags embed -o /out/oib ./cmd/server

FROM alpine:3.23
RUN apk add --no-cache ca-certificates
COPY --from=backend /out/oib /usr/local/bin/oib
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/oib"]
