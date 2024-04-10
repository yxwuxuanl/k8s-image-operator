FROM alpine:3.18

ARG CRANE_VERSION

RUN --mount=type=cache,target=/tmp \
  cd /tmp && \
  wget "https://github.com/google/go-containerregistry/releases/download/v${CRANE_VERSION}/go-containerregistry_$(uname)_$(uname -m).tar.gz -O go-containerregistry.tar.gz" -O go-containerregistry.tar.gz && \
  tar -xzf go-containerregistry.tar.gz && \
  mv go-containerregistry/crane /usr/local/bin/crane && \
  chmod +x /usr/local/bin/crane