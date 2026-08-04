// carrier-service is the deepest hop in the POC's async branch:
//
//	order-service --Kafka--> notification-service --HTTP--> shipping-service --HTTP--> carrier-service
//
// It exists to prove W3C trace context survives Kafka AND two cross-language
// hops (Java -> Go -> Go). The business logic is deliberately trivial.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("carrier-service")

// Simulated carrier options — costs in IDR, matching the Gunpla shop theme.
var carriers = []struct {
	Name    string
	BaseIDR float64
	EtaDays int
}{
	{"JNE Express", 18000, 2},
	{"SiCepat REG", 15000, 3},
	{"Anteraja Next Day", 22000, 1},
	{"DHL Express", 95000, 1},
}

type rateRequest struct {
	OrderID  string `json:"orderId"`
	Sku      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type rateResponse struct {
	Carrier string  `json:"carrier"`
	CostIDR float64 `json:"costIdr"`
	EtaDays int     `json:"etaDays"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func main() {
	// JSON to stdout, so `kubectl logs` matches the Java services' format (D5).
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	shutdownTracing, err := initTracing(context.Background())
	if err != nil {
		slog.Error("failed to initialise tracing", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	// Only the business endpoint is traced. /healthz is deliberately left
	// un-instrumented: kubelet probes it every few seconds and would otherwise
	// flood Tempo with probe spans (the Java services DO trace their
	// /q/health/* endpoints — you can see that noise in their metrics).
	mux.Handle("POST /rates", otelhttp.NewHandler(http.HandlerFunc(handleRates), "POST /rates"))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("carrier-service listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server failed", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	// Graceful shutdown matters here: without flushing the BatchSpanProcessor
	// the last requests' spans are dropped on redeploy, which looks exactly
	// like a broken trace when you go looking for them in Tempo.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown failed", "err", err)
	}
	if err := shutdownTracing(shutdownCtx); err != nil {
		slog.Error("tracer shutdown failed", "err", err)
	}
	slog.Info("carrier-service stopped")
}

func handleRates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req rateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger(ctx).Warn("rate request rejected: invalid body", "err", err)
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	logger(ctx).Info("rate requested",
		"orderId", req.OrderID, "sku", req.Sku, "quantity", req.Quantity)

	// Manual child span — the Go counterpart to @WithSpan in order-service.
	ctx, span := tracer.Start(ctx, "quote-carrier-rate")
	defer span.End()

	// Simulated upstream carrier latency, so the span has visible width in the
	// Tempo waterfall rather than collapsing to a sliver.
	time.Sleep(time.Duration(30+rand.Intn(70)) * time.Millisecond)

	pick := carriers[rand.Intn(len(carriers))]
	quantity := req.Quantity
	if quantity < 1 {
		quantity = 1
	}
	cost := pick.BaseIDR + float64(quantity-1)*5000

	span.SetAttributes(
		attribute.String("carrier.name", pick.Name),
		attribute.Float64("carrier.cost_idr", cost),
		attribute.Int("carrier.eta_days", pick.EtaDays),
	)
	logger(ctx).Info("rate quoted",
		"orderId", req.OrderID, "carrier", pick.Name, "costIdr", cost, "etaDays", pick.EtaDays)

	writeJSON(w, http.StatusOK, rateResponse{
		Carrier: pick.Name,
		CostIDR: cost,
		EtaDays: pick.EtaDays,
	})
}

// logger returns a slog.Logger carrying trace_id/span_id, so `kubectl logs`
// output correlates with traces the same way the Quarkus services' JSON logs
// do via MDC (D5).
func logger(ctx context.Context) *slog.Logger {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return slog.Default()
	}
	return slog.Default().With(
		slog.String("trace_id", sc.TraceID().String()),
		slog.String("span_id", sc.SpanID().String()),
	)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("failed to write response", "err", err)
	}
}
