// shipping-service is the first Go hop in the POC's async branch:
//
//	order-service --Kafka--> notification-service --HTTP--> shipping-service --HTTP--> carrier-service
//	                          [Java]                         [Go, here]                [Go]
//
// It receives a shipment request from the (Java) Kafka consumer, asks
// carrier-service for a rate, and returns a shipment. The business logic is
// deliberately trivial — the point is that the trace context survives Kafka
// and then two cross-language hops.
package main

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var (
	tracer  = otel.Tracer("shipping-service")
	carrier *carrierClient
	db      *sql.DB
)

type shipmentRequest struct {
	OrderID  string `json:"orderId"`
	Sku      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type shipmentResponse struct {
	ShipmentID string  `json:"shipmentId"`
	OrderID    string  `json:"orderId"`
	Carrier    string  `json:"carrier"`
	CostIDR    float64 `json:"costIdr"`
	EtaDays    int     `json:"etaDays"`
	Status     string  `json:"status"`
}

// mirrors carrier-service's request/response shapes
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

	carrierURL := os.Getenv("CARRIER_SERVICE_URL")
	if carrierURL == "" {
		carrierURL = "http://localhost:8084" // local dev default
	}
	carrier = newCarrierClient(carrierURL)
	slog.Info("carrier-service endpoint configured", "url", carrierURL)

	db, err = openDB()
	if err != nil {
		slog.Error("failed to open database", "err", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()
	slog.Info("database connection established")

	mux := http.NewServeMux()
	// Only the business endpoints are traced — /healthz is left un-instrumented
	// so kubelet probes don't flood Tempo with noise.
	mux.Handle("POST /shipments", otelhttp.NewHandler(http.HandlerFunc(handleShipments), "POST /shipments"))
	mux.Handle("GET /shipments/{orderId}", otelhttp.NewHandler(http.HandlerFunc(handleGetShipment), "GET /shipments/{orderId}"))
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
		slog.Info("shipping-service listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server failed", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	// Flush pending spans on shutdown, else the last requests' spans are lost
	// on redeploy and look like broken traces.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown failed", "err", err)
	}
	if err := shutdownTracing(shutdownCtx); err != nil {
		slog.Error("tracer shutdown failed", "err", err)
	}
	slog.Info("shipping-service stopped")
}

func handleShipments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req shipmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger(ctx).Warn("shipment request rejected: invalid body", "err", err)
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if req.OrderID == "" {
		logger(ctx).Warn("shipment request rejected: missing orderId")
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "orderId is required"})
		return
	}
	logger(ctx).Info("shipment requested",
		"orderId", req.OrderID, "sku", req.Sku, "quantity", req.Quantity)

	// Outbound hop to carrier-service. otelhttp creates the client span and
	// injects traceparent, so carrier-service continues this same trace.
	rate, err := carrier.quote(ctx, rateRequest{
		OrderID:  req.OrderID,
		Sku:      req.Sku,
		Quantity: req.Quantity,
	})
	if err != nil {
		logger(ctx).Error("carrier quote failed", "orderId", req.OrderID, "err", err)
		span := trace.SpanFromContext(ctx)
		span.RecordError(err)
		span.SetStatus(codes.Error, "carrier quote failed")
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "carrier quote failed"})
		return
	}

	// Manual child span — the Go counterpart to @WithSpan in order-service.
	ctx, span := tracer.Start(ctx, "create-shipment")
	defer span.End()

	shipmentID := newShipmentID()
	span.SetAttributes(
		attribute.String("shipment.id", shipmentID),
		attribute.String("shipment.order_id", req.OrderID),
		attribute.String("shipment.carrier", rate.Carrier),
	)

	record := shipmentRecord{
		ShipmentID: shipmentID,
		OrderID:    req.OrderID,
		Carrier:    rate.Carrier,
		CostIDR:    rate.CostIDR,
		EtaDays:    rate.EtaDays,
		Status:     "SHIPPED",
	}
	// Persist so the confirmation page can read this back. The INSERT shows up
	// as its own db.* span (otelsql), nested under create-shipment.
	if err := insertShipment(ctx, db, record); err != nil {
		logger(ctx).Error("failed to persist shipment",
			"shipmentId", shipmentID, "orderId", req.OrderID, "err", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "persist shipment failed")
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to persist shipment"})
		return
	}

	logger(ctx).Info("shipment created",
		"shipmentId", shipmentID, "orderId", req.OrderID,
		"carrier", rate.Carrier, "costIdr", rate.CostIDR, "etaDays", rate.EtaDays)

	writeJSON(w, http.StatusCreated, shipmentResponse{
		ShipmentID: shipmentID,
		OrderID:    req.OrderID,
		Carrier:    rate.Carrier,
		CostIDR:    rate.CostIDR,
		EtaDays:    rate.EtaDays,
		Status:     "SHIPPED",
	})
}

// handleGetShipment is the read path the confirmation page polls (via
// order-service). A 404 here is the normal "async branch hasn't finished yet"
// state, not an error.
func handleGetShipment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orderID := r.PathValue("orderId")

	record, err := findShipmentByOrder(ctx, db, orderID)
	if err != nil {
		logger(ctx).Error("shipment lookup failed", "orderId", orderID, "err", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "shipment lookup failed"})
		return
	}
	if record == nil {
		logger(ctx).Info("no shipment yet for order", "orderId", orderID)
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "no shipment for order " + orderID})
		return
	}

	logger(ctx).Info("shipment found",
		"shipmentId", record.ShipmentID, "orderId", orderID, "carrier", record.Carrier)

	writeJSON(w, http.StatusOK, shipmentResponse{
		ShipmentID: record.ShipmentID,
		OrderID:    record.OrderID,
		Carrier:    record.Carrier,
		CostIDR:    record.CostIDR,
		EtaDays:    record.EtaDays,
		Status:     record.Status,
	})
}

func newShipmentID() string {
	b := make([]byte, 6)
	if _, err := crand.Read(b); err != nil {
		return fmt.Sprintf("SHP-%d", time.Now().UnixNano())
	}
	return "SHP-" + strings.ToUpper(hex.EncodeToString(b))
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
