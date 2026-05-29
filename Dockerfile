# syntax=docker/dockerfile:1.7
# feedflowのマルチステージビルドです。embed同梱の単一バイナリをdistrolessのnonrootで動かします。

# ---- build stage ----
FROM golang:1.25-bookworm AS build

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=arm64

WORKDIR /src

# 依存解決を先に行いレイヤキャッシュを効かせます。
COPY go.mod go.sum* ./
RUN go mod download

# ソースとembed対象を投入します。
COPY . .

# 静的バイナリを生成します。CGO_ENABLED=0でdistroless上でも動きます。
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/feedflow ./cmd/feedflow

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# データディレクトリはEBSをマウントするため空で用意します。nonrootが書き込めるようにします。
COPY --from=build --chown=nonroot:nonroot /out/feedflow /app/feedflow

USER nonroot:nonroot

EXPOSE 8080

ENV FEEDFLOW_ADDR=:8080
ENV FEEDFLOW_DATA_DIR=/data

ENTRYPOINT ["/app/feedflow"]
