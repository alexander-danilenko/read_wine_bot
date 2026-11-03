# Web: build the Mini App. Only the built files leave this stage; npm and node_modules do not.
# Node 26 LTS slim (glibc, so the lockfile's native packages match Ubuntu CI).
# Runs on the builder's platform: the output is static files, the same for every target.
FROM --platform=$BUILDPLATFORM node:26-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/ ./
RUN npm run build

# Build: pure Go, so CGO stays off and the binary is static. It embeds the web build.
# Runs on the builder's platform and cross-compiles, so a multi-arch build needs no emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webapp/dist ./internal/webapp/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /bot .

# Run: distroless static ships CA certs (Telegram HTTPS) and a nonroot user.
FROM gcr.io/distroless/static-debian12:nonroot
# Links the published GHCR package to this repository.
LABEL org.opencontainers.image.source=https://github.com/alexander-danilenko/read_wine_bot
COPY --from=build /bot /bot
EXPOSE 8080
ENTRYPOINT ["/bot"]
