FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/loaf ./cmd/loaf

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/loaf /usr/local/bin/loaf
EXPOSE 8443
VOLUME ["/data"]
ENV LOAF_SYNC_DB=/data/sync.sqlite
ENTRYPOINT ["/usr/local/bin/loaf", "serve", "--listen", ":8443", "--db", "/data/sync.sqlite"]
