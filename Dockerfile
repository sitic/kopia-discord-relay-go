# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /relay .

# ---- runtime stage ----
FROM scratch
# CA bundle so the relay can TLS-verify discord.com
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /relay /relay

USER 65534:65534
EXPOSE 8199
ENTRYPOINT ["/relay"]
