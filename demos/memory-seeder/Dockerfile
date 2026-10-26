# Build context: repository root.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/memory-seeder ./demos/memory-seeder

# Alpine rather than distroless: Omnia's own dev chart (omnia-dev-content,
# memory-demo-seed Job) runs this image with a /bin/sh wrapper that execs
# /usr/local/bin/memory-seeder after resolving the workspace and agent UIDs.
# The ENTRYPOINT still lets this repo's chart pass flags as plain args.
FROM alpine:3.22
COPY --from=build /out/memory-seeder /usr/local/bin/memory-seeder
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/memory-seeder"]
