# Build from the exported standalone relay repository.
FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY internal ./internal
COPY cmd ./cmd
RUN CGO_ENABLED=0 go test ./... && \
    CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/nanolathe-server ./cmd/nanolathe-server

FROM scratch
COPY --from=build /out/nanolathe-server /nanolathe-server
USER 65532:65532
# 8080 serves the relay; 8081 answers only the platform health check.
EXPOSE 8080 8081
STOPSIGNAL SIGTERM
ENTRYPOINT ["/nanolathe-server"]
# Image defaults for the 512 MiB instance; app settings may override them.
ENV GOMEMLIMIT=384MiB GOMAXPROCS=1
CMD ["--listen=:8080", "--websocket", "--behind-tls-proxy", "--max-rooms=128", "--max-connections=512", "--health-listen=:8081"]
