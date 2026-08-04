# Stage 1 — compile a static binary. Tags pinned exactly per D7.
FROM golang:1.26.5-alpine3.24 AS build
WORKDIR /src
# go.sum isn't committed (no Go toolchain on the authoring machine), so deps
# are resolved here; direct versions are pinned in go.mod. `go vet` runs as a
# build gate since we can't compile-check before this point.
COPY go.mod ./
COPY *.go ./
RUN go mod tidy \
 && go vet ./... \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/carrier-service .

# Stage 2 — tiny runtime. alpine rather than scratch/distroless so there's a
# shell for `kubectl exec` debugging, which has repeatedly earned its keep on
# this cluster.
FROM alpine:3.24.1
RUN adduser -D -u 10001 app
COPY --from=build /out/carrier-service /usr/local/bin/carrier-service
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/carrier-service"]
