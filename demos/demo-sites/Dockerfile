# Build context: repo root. Build with:
#   docker build -f demos/demo-sites/Dockerfile -t omnia-demo-sites:dev .
# Run one brand with:
#   docker run -p 8080:8080 omnia-demo-sites:dev --site wrenmoor-energy
#
# Every brand's site is embedded into the one binary (go:embed in server.go),
# so there is no volume, no COPY of static assets into the final image, and
# nothing to keep in step at runtime. --site picks the brand.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
    go build -o /out/demo-sites ./demos/demo-sites

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/demo-sites /demo-sites
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/demo-sites"]
