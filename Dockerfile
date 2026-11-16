# Mila as a single static binary on a distroless base.
#   podman build -t mila .   (or docker build)
#
# The web and Go stages run on the building machine and Go cross-compiles,
# so an image for another architecture (docker buildx --platform
# linux/arm64) needs no emulation.

FROM --platform=$BUILDPLATFORM docker.io/library/node:24-alpine@sha256:a34ab6d7f6f4813f4d0c5469d74685604b962fbdbf5793a7bc7b4998ecb867c8 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY shared/ /src/shared/
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.26-alpine@sha256:30b1e546a9c7fa3b747c2bdba3a0c7bdf3a02ebb4c5b8405d2f948d7e8aac4ec AS server
ARG VERSION=dev
ARG COMMIT=unknown
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /src/web/build/ ./internal/webui/dist/
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X github.com/locodarwin/mila/server/internal/version.Version=${VERSION} -X github.com/locodarwin/mila/server/internal/version.Commit=${COMMIT}" \
    -o /out/mila ./cmd/mila
# An empty data folder for the image, so a new volume mounted there
# starts out owned by the user Mila runs as (distroless nonroot, 65532).
RUN mkdir /out/data

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=server /out/mila /usr/local/bin/mila
COPY LICENSE /usr/share/doc/mila/LICENSE
COPY --from=server --chown=65532:65532 /out/data /data
ENV MILA_LISTEN=:8080 \
    MILA_DATA_DIR=/data
VOLUME /data
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/mila"]
CMD ["serve"]
