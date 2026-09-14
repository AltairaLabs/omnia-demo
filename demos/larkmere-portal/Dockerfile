# Build context: repo root. Build with:
#   docker build -f demos/larkmere-portal/Dockerfile -t omnia-larkmere-portal:dev .
#
# The site is embedded into the binary (go:embed in main.go), so there is no
# volume, no COPY of static assets into the final image, and nothing to keep in
# step at runtime.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
    go build -o /out/larkmere-portal ./demos/larkmere-portal

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/larkmere-portal /larkmere-portal
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/larkmere-portal"]
