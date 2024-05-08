FROM alpine:3.18

ARG CRANE_VERSION
ARG OS=linux
ARG ARCH=x86_64

RUN --mount=type=cache,target=/tmp \
  wget https://github.com/google/go-containerregistry/releases/download/v${CRANE_VERSION}/go-containerregistry_${OS}_${ARCH}.tar.gz -O /tmp/go-containerregistry.tar.gz && \
  tar -zxvf /tmp/go-containerregistry.tar.gz -C /usr/local/bin/ crane

ENTRYPOINT ["/usr/local/bin/crane"]