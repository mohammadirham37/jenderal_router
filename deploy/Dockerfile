# JenderalRouter — image produksi multi-arch (linux/amd64 + linux/arm64, NFR-15).
# Build:  docker build -f deploy/Dockerfile -t jenderalrouter .
# Build UI di-embed otomatis (internal/api/static via embed.FS).

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETARCH
ARG TARGETOS=linux
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X github.com/jenderal/jenderalrouter/internal/api.Version=${VERSION}" \
    -o /out/jenderalrouter ./cmd/jenderalrouter

FROM scratch
LABEL org.opencontainers.image.title="JenderalRouter"
LABEL org.opencontainers.image.description="Gateway LLM self-hosted multi-user (OpenAI + Anthropic compatible)"
COPY --from=build /out/jenderalrouter /jenderalrouter
# user non-root; data dir ditulis via volume
USER 65534:65534
ENV JR_ADDR=0.0.0.0:20130 \
    JR_DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 20130
HEALTHCHECK --interval=30s --timeout=3s CMD ["/jenderalrouter", "-healthcheck"]
ENTRYPOINT ["/jenderalrouter"]
