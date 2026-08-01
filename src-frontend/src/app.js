import { SpanStatusCode } from '@opentelemetry/api';
import { tracer, API_BASE_URL } from './otel.js';

const form = document.getElementById('order-form');
const resultEl = document.getElementById('result');

form.addEventListener('submit', (event) => {
  event.preventDefault();
  const sku = document.getElementById('sku').value.trim();
  const quantity = parseInt(document.getElementById('quantity').value, 10);
  resultEl.textContent = 'Placing order…';

  // Manual root span for the user interaction (PRD 5.4 / S4-style, browser side).
  // startActiveSpan makes it the active span, so the auto fetch span nests under it.
  tracer.startActiveSpan('place-order', async (span) => {
    span.setAttribute('app.sku', sku);
    span.setAttribute('app.quantity', quantity);
    try {
      const res = await fetch(`${API_BASE_URL}/orders`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ sku, quantity }),
      });
      const body = await res.json().catch(() => ({}));
      span.setAttribute('http.status_code', res.status);
      if (res.ok) {
        resultEl.textContent = `✅ ${res.status} — order ${body.orderId} (${body.status})`;
      } else {
        span.setStatus({ code: SpanStatusCode.ERROR, message: `HTTP ${res.status}` });
        resultEl.textContent = `⚠️ ${res.status} — ${body.error || JSON.stringify(body)}`;
      }
    } catch (err) {
      span.recordException(err);
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(err) });
      resultEl.textContent = `❌ ${err}`;
    } finally {
      // Print this trace ID so you can find the exact trace in Grafana
      const traceId = span.spanContext().traceId;
      resultEl.textContent += `\ntraceId: ${traceId}`;
      span.end();
    }
  });
});
