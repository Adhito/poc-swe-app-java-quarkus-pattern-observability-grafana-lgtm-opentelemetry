module poc.tracing/shipping-service

go 1.26

// Direct dependencies pinned per D7. go.sum is not committed (no Go toolchain
// on the authoring machine) — `go mod tidy` runs inside the Docker build to
// resolve indirect deps. See the runbook's D7 note.
require (
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0
	go.opentelemetry.io/otel v1.45.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.45.0
	go.opentelemetry.io/otel/sdk v1.45.0
	go.opentelemetry.io/otel/trace v1.45.0
)
