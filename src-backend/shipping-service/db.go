package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"go.opentelemetry.io/otel/attribute"
)

// sqlSpanName turns a statement into a Java-JDBC-style span name
// ("INSERT shipments", "SELECT shipments"), so Go and Java DB spans are
// directly comparable in the trace waterfall. Returns "" if it can't tell.
func sqlSpanName(query string) string {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return ""
	}
	op := strings.ToUpper(fields[0])
	switch op {
	case "INSERT", "DELETE":
		keyword := "INTO"
		if op == "DELETE" {
			keyword = "FROM"
		}
		if table := tokenAfter(fields, keyword); table != "" {
			return op + " " + table
		}
	case "SELECT":
		if table := tokenAfter(fields, "FROM"); table != "" {
			return op + " " + table
		}
	case "UPDATE":
		if len(fields) > 1 {
			return op + " " + trimIdentifier(fields[1])
		}
	}
	return op
}

func tokenAfter(fields []string, keyword string) string {
	for i, field := range fields {
		if strings.EqualFold(field, keyword) && i+1 < len(fields) {
			return trimIdentifier(fields[i+1])
		}
	}
	return ""
}

func trimIdentifier(s string) string {
	return strings.Trim(s, "\"`(),;")
}

type shipmentRecord struct {
	ShipmentID string
	OrderID    string
	Carrier    string
	CostIDR    float64
	EtaDays    int
	Status     string
}

// openDB returns an instrumented *sql.DB.
//
// otelsql wraps database/sql so every statement emits a db.* span carrying
// db.statement — the Go counterpart to `quarkus.datasource.jdbc.telemetry=true`
// in stock-service. That gives S2-style SQL visibility in a second language.
func openDB() (*sql.DB, error) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://stock:stock@localhost:5432/stock?sslmode=disable"
	}

	db, err := otelsql.Open("pgx", dsn,
		otelsql.WithAttributes(attribute.String("db.system", "postgresql")),
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			// Connection-lifecycle spans (reset_session, prepare, rows) are
			// pure noise in the waterfall — we only care about statements.
			OmitConnResetSession: true,
			OmitConnPrepare:      true,
			OmitRows:             true,
			OmitConnectorConnect: true,
		}),
		// Java's JDBC instrumentation names spans after the statement
		// ("INSERT stock.orders"); otelsql defaults to the connection method
		// ("sql.conn.exec"), which tells you nothing about the query. Derive a
		// comparable name so the Go DB spans read like the Java ones in Tempo.
		otelsql.WithSpanNameFormatter(func(_ context.Context, method otelsql.Method, query string) string {
			if name := sqlSpanName(query); name != "" {
				return name
			}
			return string(method)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Small pool — this service handles one request per order.
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return db, nil
}

func insertShipment(ctx context.Context, db *sql.DB, s shipmentRecord) error {
	const q = `INSERT INTO shipments (shipment_id, order_id, carrier, cost_idr, eta_days, status)
	           VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := db.ExecContext(ctx, q,
		s.ShipmentID, s.OrderID, s.Carrier, s.CostIDR, s.EtaDays, s.Status); err != nil {
		return fmt.Errorf("insert shipment: %w", err)
	}
	return nil
}

// findShipmentByOrder returns (nil, nil) when there's no shipment yet — the
// normal state while the async branch is still in flight.
func findShipmentByOrder(ctx context.Context, db *sql.DB, orderID string) (*shipmentRecord, error) {
	const q = `SELECT shipment_id, order_id, carrier, cost_idr, eta_days, status
	           FROM shipments WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`
	var s shipmentRecord
	err := db.QueryRowContext(ctx, q, orderID).Scan(
		&s.ShipmentID, &s.OrderID, &s.Carrier, &s.CostIDR, &s.EtaDays, &s.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select shipment: %w", err)
	}
	return &s, nil
}
