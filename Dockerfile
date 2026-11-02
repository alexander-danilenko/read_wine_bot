# Web: build the Mini App. Only the built files leave this stage; npm and node_modules do not.
# Pinned by digest: node 26 LTS slim (glibc, so the lockfile's native packages match Ubuntu CI).
FROM node:26-slim@sha256:e4db4c914bf8ad241bd24804fc25d272d89bad97e68cdc8a1414862790a02c39 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/ ./
RUN npm run build

# Build: pure Go, so CGO stays off and the binary is static. It embeds the web build.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webapp/dist ./internal/webapp/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bot .

# Run: distroless static ships CA certs (Telegram HTTPS) and a nonroot user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bot /bot
EXPOSE 8080
ENTRYPOINT ["/bot"]
