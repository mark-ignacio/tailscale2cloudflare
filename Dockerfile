FROM golang:1.17 as build
WORKDIR /app
COPY . .
RUN go build -o /tailscale2cloudflare

FROM gcr.io/distroless/base-debian10
COPY --from=build /tailscale2cloudflare /tailscale2cloudflare
ENTRYPOINT [ "/tailscale2cloudflare" ]