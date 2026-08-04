package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"go.opentelemetry.io/otel/attribute"
)

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
