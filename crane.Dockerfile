ARG CRANE_VERSION
ARG HTTPS_PROXY

FROM alpine:3.18

RUN --mount=type=cache,target=/tmp \
  cd /tmp && \
  HTTPS_PROXY=$(HTTPS_PROXY) wget "https://github.com/google/go-containerregistry/releases/download/v$(CRANE_VERSION)/go-containerregistry_$(uname)_$(uname -m).tar.gz" -O go-containerregistry.tar.gz && \
  tar -xzf go-containerregistry.tar.gz && \
  mv go-containerregistry/crane /usr/local/bin/crane && \
  chmod +x /usr/local/bin/crane