FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY core ./core
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/bili-ip-anchor .

# distroless/static 自带 CA 证书，没有 shell，体积约 2 MB
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/bili-ip-anchor /usr/local/bin/bili-ip-anchor
ENV DATA_DIR=/data \
    TZ=Asia/Shanghai
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["bili-ip-anchor"]
CMD ["daemon"]
