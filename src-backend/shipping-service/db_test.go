package main

import "testing"

// Guards the span naming used in Tempo — the whole point is that Go DB spans
// read like the Java ones ("INSERT shipments", not "sql.conn.exec"), so a
// regression here is silently confusing rather than loud.
func TestSQLSpanName(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{
			name: "insert matches the real statement in this service",
			query: `INSERT INTO shipments (shipment_id, order_id, carrier, cost_idr, eta_days, status)
			        VALUES ($1, $2, $3, $4, $5, $6)`,
			want: "INSERT shipments",
		},
		{
			name: "select matches the real lookup in this service",
			query: `SELECT shipment_id, order_id, carrier, cost_idr, eta_days, status
			        FROM shipments WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`,
			want: "SELECT shipments",
		},
		{name: "update", query: "UPDATE shipments SET status = $1", want: "UPDATE shipments"},
		{name: "delete", query: "DELETE FROM shipments WHERE order_id = $1", want: "DELETE shipments"},
		{name: "lowercase keywords", query: "select * from shipments", want: "SELECT shipments"},
		{name: "unknown statement falls back to the verb", query: "BEGIN", want: "BEGIN"},
		{name: "empty query yields no name", query: "   ", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sqlSpanName(tc.query); got != tc.want {
				t.Errorf("sqlSpanName(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}
