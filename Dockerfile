FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN go build -trimpath -ldflags="-s -w" -o /out/dnshe-ddns-go-callback .

FROM alpine:3.21
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/dnshe-ddns-go-callback /app/dnshe-ddns-go-callback
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/dnshe-ddns-go-callback /app/docker-entrypoint.sh

ENV PORT=18491
EXPOSE 18491
ENTRYPOINT ["/app/docker-entrypoint.sh"]
