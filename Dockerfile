FROM golang:1.16 as build
WORKDIR /app
COPY . .
RUN go build -o /tailscale2cloudflare

FROM gcr.io/distroless/static-debian10
COPY --from=build /tailscale2cloudflare /
ENTRYPOINT [ "/tailscale2cloudflare" ]