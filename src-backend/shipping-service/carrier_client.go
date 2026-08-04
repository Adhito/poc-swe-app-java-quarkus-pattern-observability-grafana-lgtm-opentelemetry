package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type carrierClient struct {
	baseURL string
	http    *http.Client
}

func newCarrierClient(baseURL string) *carrierClient {
	return &carrierClient{
		baseURL: baseURL,
		http: &http.Client{
			// otelhttp.NewTransport is what injects the `traceparent` header
			// into the outgoing request and creates the client span. It's the
			// explicit equivalent of what the MicroProfile REST client does
			// automatically over in order-service — same result, but here you
			// can see the wiring.
			Transport: otelhttp.NewTransport(http.DefaultTransport),
			Timeout:   10 * time.Second,
		},
	}
}

func (c *carrierClient) quote(ctx context.Context, req rateRequest) (*rateResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal rate request: %w", err)
	}

	// NewRequestWithContext (not NewRequest) is essential: the context carries
	// the active span, and the otelhttp transport reads it to inject
	// traceparent. With a background context the trace would break here.
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/rates", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build rate request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call carrier-service: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("carrier-service returned HTTP %d", resp.StatusCode)
	}

	var rate rateResponse
	if err := json.NewDecoder(resp.Body).Decode(&rate); err != nil {
		return nil, fmt.Errorf("decode rate response: %w", err)
	}
	return &rate, nil
}
