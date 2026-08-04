package main

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	defaultServiceName = "carrier-service"
	serviceVersion     = "1.0.0"
)

// initTracing wires up the OpenTelemetry SDK by hand.
//
// Note how much of this is explicit compared to the Quarkus services, where
// simply adding the quarkus-opentelemetry extension does all of it invisibly.
// That contrast is a large part of why these Go services exist at all
// (PRD learning objective: "auto-instrumentation vs. manual").
//
// Endpoint and service name come from OTEL_EXPORTER_OTLP_ENDPOINT and
// OTEL_SERVICE_NAME, which the Go SDK reads natively — set in the Deployment
// exactly like the Java services' env vars.
func initTracing(ctx context.Context) (func(context.Context) error, error) {
	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}

	// Raw attribute keys rather than the semconv package: semconv's import
	// path is version-stamped (.../semconv/v1.xx.0) and moves between
	// releases, so this avoids coupling the build to one specific revision.
	attrs := []attribute.KeyValue{
		attribute.String("service.version", serviceVersion),
	}
	if os.Getenv("OTEL_SERVICE_NAME") == "" {
		// Otherwise the SDK reports "unknown_service:<binary>" in Tempo.
		attrs = append(attrs, attribute.String("service.name", defaultServiceName))
	}

	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(attrs...))
	if err != nil {
		return nil, fmt.Errorf("resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// ⚠️ THE critical line for cross-language tracing.
	//
	// Go does NOT install W3C propagators by default. Without this, the
	// incoming `traceparent` header is ignored and this service silently
	// starts a BRAND NEW trace instead of continuing the caller's — no error,
	// no warning, nothing in the logs. The only symptom is a split trace in
	// Tempo. Same family of silent-config trap as the two Quarkus property
	// bugs already in documents/CLAUDE-CODE-FIXING-LOG.md.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
