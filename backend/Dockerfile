# The build stage runs on the builder's own architecture and cross-compiles,
# so multi-architecture images build quickly instead of under emulation.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X github.com/Kayamii/Podora/backend/internal/api.Version=${VERSION}" \
    -o /out/podora ./cmd/podora

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/podora /podora
EXPOSE 8080
ENTRYPOINT ["/podora"]
