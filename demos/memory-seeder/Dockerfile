# Build context: repository root.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/memory-seeder ./demos/memory-seeder

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/memory-seeder /memory-seeder
USER nonroot:nonroot
ENTRYPOINT ["/memory-seeder"]
