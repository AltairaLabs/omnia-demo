FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/memory-ingestion .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/memory-ingestion /memory-ingestion
ENTRYPOINT ["/memory-ingestion"]
