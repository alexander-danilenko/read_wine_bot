# Build: pure Go, so CGO stays off and the binary is static.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bot .

# Run: distroless static ships CA certs (Telegram HTTPS) and a nonroot user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bot /bot
ENTRYPOINT ["/bot"]
