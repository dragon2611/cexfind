# build the webapp including dependencies from / and /cmd

# For information about the use of the gcr.io/distroless/static final
# container image, see the gcr.io docs at
# https://github.com/GoogleContainerTools/distroless/blob/main/base/README.md

FROM --platform=$BUILDPLATFORM golang:1.27 AS deps

# setup module environment
WORKDIR /build
ADD go.mod go.sum ./
RUN go mod download
ADD cmd/web/*mod cmd/web/*sum ./cmd/web/
RUN cd /build/cmd/web && go mod download

# build
FROM deps AS dev
ARG TARGETARCH
ARG BUILD_OS=linux
ADD *go ./
ADD cmd/*go ./cmd/
ADD cmd/web/ ./cmd/web/
ADD location/ ./location/
RUN cd cmd/web && \
    CGO_ENABLED=0 GOOS=${BUILD_OS} GOARCH=${TARGETARCH} \
    go build -ldflags "-w -X main.docker=true" -o /build/webserver .

# Export a binary with `docker build --target binary --output`.
FROM scratch AS binary
COPY --from=dev /build/webserver /webserver

# Build and export all three clients for the requested target OS/architecture.
FROM dev AS clients
ARG TARGETARCH
ARG BUILD_OS=linux
ADD cmd/cli/ ./cmd/cli/
ADD cmd/console/ ./cmd/console/
RUN cd cmd/cli && go mod download && \
    CGO_ENABLED=0 GOOS=${BUILD_OS} GOARCH=${TARGETARCH} \
    go build -ldflags "-w" -o /build/cli .
RUN cd cmd/console && go mod download && \
    CGO_ENABLED=0 GOOS=${BUILD_OS} GOARCH=${TARGETARCH} \
    go build -ldflags "-w" -o /build/console .

FROM scratch AS all-binaries
COPY --from=clients /build/webserver /webserver
COPY --from=clients /build/cli /cli
COPY --from=clients /build/console /console

# install into minimal image
FROM gcr.io/distroless/base AS base
WORKDIR /
EXPOSE 8000
COPY --from=dev /build/webserver /
CMD ["/webserver", "--address", "0.0.0.0", "--port", "8000"]
