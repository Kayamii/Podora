FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/Kayamii/Podora/backend/internal/api.Version=${VERSION}" \
    -o /out/podora ./cmd/podora

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/podora /podora
EXPOSE 8080
ENTRYPOINT ["/podora"]
