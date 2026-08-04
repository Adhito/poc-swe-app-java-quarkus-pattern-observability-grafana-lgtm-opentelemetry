package main

// NOTE: this file is intentionally a near-copy of carrier-service/otel.go.
// The two services are separate Go modules, and sharing it would mean either a
// third shared module or `replace` directives — more build machinery than two
// ~70-line files are worth for a POC.

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
	defaultServiceName = "shipping-service"
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
	// incoming `traceparent` header from notification-service is ignored and
	// this service silently starts a BRAND NEW trace — no error, no warning.
	// The only symptom is a split trace in Tempo. It also breaks the OUTGOING
	// direction: otelhttp.NewTransport uses this same propagator to inject
	// traceparent into the call to carrier-service.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
