# Build context: repo root. Build with:
#   docker build -f demos/personal-agent/Dockerfile -t omnia-demo-personal-agent:dev .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
    go build -o /out/personal-agent ./demos/personal-agent

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/personal-agent /personal-agent
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/personal-agent"]
