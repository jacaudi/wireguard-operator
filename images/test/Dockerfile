FROM golang:1.26

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git curl bash && rm -rf /var/lib/apt/lists/*

# setup-envtest to fetch apiserver/etcd assets.
# PINNED, not @latest: an unpinned install makes this image's contents depend on
# the day it was built, which is the opposite of what a test image is for — and
# it silently drifts away from the controller-runtime the module actually uses.
# Keep in step with the sigs.k8s.io/controller-runtime version in go.mod.
# renovate: datasource=go depName=sigs.k8s.io/controller-runtime
ARG CONTROLLER_RUNTIME_VERSION=v0.18.5
RUN go install "sigs.k8s.io/controller-runtime/tools/setup-envtest@${CONTROLLER_RUNTIME_VERSION}"

WORKDIR /workspace

# Warm module cache (optional)
COPY go.mod go.sum ./
RUN go mod download

# We run tests with bind-mounted source, not copying sources into the image

# No entrypoint; caller will invoke /bin/bash explicitly

