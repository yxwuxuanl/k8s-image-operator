FROM alpine:3.18

ARG TARGETOS
ARG TARGETARCH

ARG CRANE_VERSION

RUN --mount=type=cache,target=/tmop <<EOF
  wget -Yon https://github.com/google/go-containerregistry/releases/download/v$CRANE_VERSION/go-containerregistry_${TARGETOS}_${TARGETARCH}.tar.gz -O /tmp/go-containerregistry.tar.gz
  tar -zxvf /tmp/go-containerregistry.tar.gz -C /usr/local/bin/ crane
EOF

ENTRYPOINT ["/usr/local/bin/crane"]