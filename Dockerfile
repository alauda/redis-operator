FROM golang:1.20 as builder
WORKDIR /workspace

COPY . .

ENV GOPROXY=https://goproxy.io,direct
# Build
RUN CGO_ENABLED=0 go build -buildmode=pie -ldflags '-extldflags "-Wl,-z,relro,-z,now" -linkmode=external -w -s' -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
