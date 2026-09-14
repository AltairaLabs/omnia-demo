# Build context: repo root. Build with:
#   docker build -f demos/mortgage-underwriting/Dockerfile -t omnia-income-verify-stub:dev .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
    go build -o /out/income-verify ./demos/mortgage-underwriting

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/income-verify /income-verify
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/income-verify"]
