# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.26.4-bookworm AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src/server

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/xmdm-server ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app

COPY --from=build /out/xmdm-server /usr/local/bin/xmdm-server

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/xmdm-server"]
