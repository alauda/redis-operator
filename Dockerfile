FROM golang:1.20 as builder
WORKDIR /workspace

COPY . .

ENV GOPROXY=https://goproxy.io,direct
# Build
RUN CGO_ENABLED=1 go build -buildmode=pie -ldflags '-extldflags "-Wl,-z,relro,-z,now" -linkmode=external -w -s' -a -o manager cmd/main.go && go build -buildmode=pie -ldflags '-extldflags "-Wl,-z,relro,-z,now" -linkmode=external -w -s' -a -o redis-tools cmd/redis-tools/main.go

FROM redis:7.2-alpine

COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/redis-tools /opt/
COPY --from=builder /workspace/cmd/redis-tools/scripts/ /opt/

RUN apk add --no-cache gcompat

RUN ARCH= &&dpkgArch="$(arch)" \
  && case "${dpkgArch}" in \
    x86_64) ARCH='amd64';; \
    aarch64) ARCH='arm64';; \
    *) echo "unsupported architecture"; exit 1 ;; \
  esac \
  && wget https://storage.googleapis.com/kubernetes-release/release/v1.28.3/bin/linux/${ARCH}/kubectl -O /bin/kubectl && chmod +x /bin/kubectl

ENV PATH="/opt:$PATH"

USER 65532:65532

ENTRYPOINT ["/manager"]
