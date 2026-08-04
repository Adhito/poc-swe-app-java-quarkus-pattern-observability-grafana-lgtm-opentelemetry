# Stage 1 — compile a static binary. Tags pinned exactly per D7.
FROM golang:1.26.5-alpine3.24 AS build
WORKDIR /src
# Deps first, in their own layer — this only re-runs when go.mod/go.sum change,
# not on every source edit. go.sum is committed, so this verifies hashes rather
# than resolving versions at build time (D7 reproducibility).
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN go vet ./... \
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
