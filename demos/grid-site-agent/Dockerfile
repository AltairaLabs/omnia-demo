# Build context: repo root. Build with:
#   docker build -f demos/grid-site-agent/Dockerfile -t omnia-demo-grid-site-agent:dev .
#
# The on-site agent and its pack, in one image. It opens one outbound
# connection to Omnia and listens on nothing but its probe port (PORT), which
# only the local kubelet uses.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
    go build -trimpath -ldflags="-s -w" -o /out/grid-site-agent ./demos/grid-site-agent

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/grid-site-agent /grid-site-agent
# The same compiled pack the chart's PromptPack carries.
COPY charts/omnia-demo/files/arena/grid-site-agent-pack.json /pack/grid-site-agent.pack.json
USER nonroot:nonroot
ENTRYPOINT ["/grid-site-agent"]
