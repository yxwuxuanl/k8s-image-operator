FROM alpine:3.18

ARG CRANE_VERSION

RUN --mount=type=cache,target=/tmp \
  cd /tmp && \
  echo "https://github.com/google/go-containerregistry/releases/download/v${CRANE_VERSION}/go-containerregistry_$(uname)_$(uname -m).tar.gz -O go-containerregistry.tar.gz" && \
  wget "https://github.com/google/go-containerregistry/releases/download/v${CRANE_VERSION}/go-containerregistry_$(uname)_$(uname -m).tar.gz -O go-containerregistry.tar.gz" -O go-containerregistry.tar.gz && \
  tar -zxvf go-containerregistry.tar.gz -C /usr/local/bin/ crane