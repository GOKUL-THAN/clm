# ==============================================================================
# CLM vSatellite Agent - Multi-Stage Container Build
# ==============================================================================

# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git make bash ca-certificates

# Copy source tree
COPY . .

# Build staged binary
RUN cd components/vsatellite && make build

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Create unprivileged service user
RUN addgroup -S clm && adduser -S -G clm -h /opt/clm-vsatellite clm

WORKDIR /opt/clm-vsatellite

# Copy compiled binary and sample configuration
COPY --from=builder /src/components/vsatellite/bin/clm-vsatellite /usr/local/bin/clm-vsatellite
COPY --from=builder /src/components/vsatellite/configs/vsatellite.yaml /etc/clm/vsatellite.yaml

RUN chown -R clm:clm /opt/clm-vsatellite /etc/clm

USER clm

ENV CLM_VSATELLITE_CONFIG=/etc/clm/vsatellite.yaml

ENTRYPOINT ["/usr/local/bin/clm-vsatellite"]
