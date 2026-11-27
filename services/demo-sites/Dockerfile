# Build context: repo root. Build with:
#   docker build -f services/demo-sites/Dockerfile -t omnia-demo-sites:dev .
# Run one brand with:
#   docker run -p 8080:8080 omnia-demo-sites:dev --site wrenmoor-energy
#
# Every brand's site is embedded into the one binary (go:embed in server.go),
# so there is no volume, no COPY of static assets into the final image, and
# nothing to keep in step at runtime. --site picks the brand.
# Compile each UI kit's JSX into one app.js (hack/build-kits.sh), so a page
# loads one small bundle instead of Babel and a module per round trip.
FROM node:22-alpine AS kits
RUN apk add --no-cache bash
WORKDIR /src
COPY services/demo-sites/hack services/demo-sites/hack
COPY services/demo-sites/site services/demo-sites/site
RUN services/demo-sites/hack/build-kits.sh

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=kits /src/services/demo-sites/site services/demo-sites/site
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
    go build -o /out/demo-sites ./services/demo-sites && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
    go build -o /out/card-callers ./services/card-callers

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/demo-sites /demo-sites
# The card-support canary's scripted callers (services/card-callers), shipped here
# rather than as an image of their own.
COPY --from=build /out/card-callers /card-callers
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/demo-sites"]
