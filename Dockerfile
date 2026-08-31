# Multi-stage build: static binary in a distroless image, ~10 MB final,
# running as the non-root uid 65532.
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/nudge .
RUN mkdir -p /data && chown 65532:65532 /data

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="nudge" \
      org.opencontainers.image.description="Self-hosted developer notification inbox" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chown=65532:65532 /data /data
COPY --from=build /out/nudge /nudge
VOLUME ["/data"]
ENV NUDGE_ADDR=:8080 \
    NUDGE_DATA_DIR=/data
EXPOSE 8080
ENTRYPOINT ["/nudge", "serve"]
