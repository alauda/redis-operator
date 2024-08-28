# currently supported builders:
# - golang
ARG BUILDER=golang
ARG BUILDER_VERSION=1.22
ARG BUILDER_IMAGE=$BUILDER:$BUILDER_VERSION

# currently supported base image types:
# - static
# - busybox
# ARG BASE_TYPE=gcr.io/distroless/static:nonroot
ARG BASE_TYPE=busybox
ARG BASE_IMAGE=$BASE_TYPE

# stage 1: build
FROM $BUILDER_IMAGE AS builder

WORKDIR /workspace

# Copy the go source
COPY version version
COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/
COPY pkg/ pkg/
COPY tools/ tools/
COPY Makefile Makefile

# Dependencies are cached unless we change go.mod or go.sum
COPY go.mod go.mod
COPY go.sum go.sum
RUN GOPROXY=https://goproxy.cn,direct go mod download

RUN CGO_ENABLED=0 go build -a -tags timetzdata -buildmode=pie -ldflags '-extldflags "-Wl,-z,relro,-z,now"' -a -o manager cmd/main.go 
RUN CGO_ENABLED=0 go build -a -tags timetzdata -buildmode=pie -ldflags '-extldflags "-Wl,-z,relro,-z,now"' -a -o redis-tools cmd/redis-tools/main.go

# stage 2: release
FROM $BASE_IMAGE AS release

WORKDIR /

# see `ONBUILD RUN` in builder recipe for artifact output path
# use `--link` to increase cache hit; use `--chmod` to minimize file permission
COPY --link --from=builder --chmod=555 /workspace/manager .
COPY --link --from=builder --chmod=555 /workspace/redis-tools /opt/
COPY --link --from=builder --chmod=555 /workspace/tools/ /opt/

ENV PATH="/opt:$PATH"

ENTRYPOINT ["/manager"]
