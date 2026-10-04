# JenderalRouter — image produksi multi-arch (linux/amd64 + linux/arm64, NFR-15).
# Build:  docker build -f deploy/Dockerfile -t jenderalrouter .
# UI SvelteKit di-build di stage node lalu di-embed ke binary Go.

FROM --platform=$BUILDPLATFORM node:22-alpine AS ui
WORKDIR /ui
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ .
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETARCH
ARG TARGETOS=linux
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# pakai hasil build UI terbaru (menimpa dist yang ter-commit)
COPY --from=ui /ui/build ./internal/web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X github.com/jenderal/jenderalrouter/internal/api.Version=${VERSION}" \
    -o /out/jenderalrouter ./cmd/jenderalrouter

FROM scratch
LABEL org.opencontainers.image.title="JenderalRouter"
LABEL org.opencontainers.image.description="Gateway LLM self-hosted multi-user (OpenAI + Anthropic compatible)"
COPY --from=build /out/jenderalrouter /jenderalrouter
# Catatan: proses berjalan sebagai root DI DALAM container agar bisa menulis
# volume /data; port 20130 hanya di-publish ke loopback host oleh compose.
ENV JR_ADDR=0.0.0.0:20130 \
    JR_DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 20130
HEALTHCHECK --interval=30s --timeout=3s CMD ["/jenderalrouter", "-healthcheck"]
ENTRYPOINT ["/jenderalrouter"]
