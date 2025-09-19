# Use a minimal Alpine image to get the root certificates.
FROM alpine:latest AS certs

# Build minimal image from scratch
FROM scratch AS minimal

ARG TARGETOS TARGETARCH
COPY --chmod=0755 build/bin/go-smtp-slacker_${TARGETOS}_${TARGETARCH} /usr/bin/go-smtp-slacker

# Copy the CA certificates from the 'certs' stage.
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/usr/bin/go-smtp-slacker"]