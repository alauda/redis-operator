FROM golang:1.22-alpine as builder

WORKDIR /workspace

# Dependencies are cached unless we change go.mod or go.sum
COPY go.mod go.mod
COPY go.sum go.sum
RUN GOPROXY=https://goproxy.io,direct go mod download

# Copy the go source
COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/
COPY pkg/ pkg/
COPY tools/ tools/


RUN CGO_ENABLED=0 go build -a -tags timetzdata -buildmode=pie -ldflags '-extldflags "-Wl,-z,relro,-z,now"' -a -o manager cmd/main.go 
RUN CGO_ENABLED=0 go build -a -tags timetzdata -buildmode=pie -ldflags '-extldflags "-Wl,-z,relro,-z,now"' -a -o redis-tools cmd/redis-tools/main.go


FROM redis:7.2-alpine

RUN apk add --no-cache gcompat

COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/redis-tools /opt/
COPY --from=builder /workspace/tools/ /opt/

ENV PATH="/opt:$PATH"

USER 65532:65532

ENTRYPOINT ["/manager"]
