import { SpanStatusCode } from '@opentelemetry/api';
import { tracer, API_BASE_URL } from './otel.js';

const POLL_INTERVAL_MS = 1000;
const MAX_POLLS = 15;

const form = document.getElementById('order-form');
const resultEl = document.getElementById('result');
const orderView = document.getElementById('order-view');
const confirmView = document.getElementById('confirm-view');

const idr = new Intl.NumberFormat('id-ID', {
  style: 'currency',
  currency: 'IDR',
  maximumFractionDigits: 0,
});
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const setText = (id, text) => {
  document.getElementById(id).textContent = text;
};

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
    let placed = null;
    try {
      const res = await fetch(`${API_BASE_URL}/orders`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ sku, quantity }),
      });
      const body = await res.json().catch(() => ({}));
      span.setAttribute('http.status_code', res.status);
      if (res.ok) {
        placed = body;
      } else {
        span.setStatus({ code: SpanStatusCode.ERROR, message: `HTTP ${res.status}` });
        resultEl.textContent = `⚠️ ${res.status} — ${body.error || JSON.stringify(body)}`;
      }
    } catch (err) {
      span.recordException(err);
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(err) });
      resultEl.textContent = `❌ ${err}`;
    } finally {
      const traceId = span.spanContext().traceId;
      if (!placed) {
        resultEl.textContent += `\ntraceId: ${traceId}`;
      }
      span.end();
      // Switch views only after the span has closed, so the confirmation
      // screen's polling starts its own trace instead of extending this one.
      if (placed) {
        showConfirmation(placed, traceId);
        awaitShipment(placed.orderId);
      }
    }
  });
});

document.getElementById('new-order').addEventListener('click', () => {
  confirmView.hidden = true;
  orderView.hidden = false;
  resultEl.textContent = '';
});

function showConfirmation(order, traceId) {
  setText('c-order-id', order.orderId);
  setText('c-sku', order.sku);
  setText('c-qty', String(order.quantity));
  setText('c-trace', traceId);
  setStatus('Shipping pending', 'pending');
  document.getElementById('shipment-block').hidden = true;
  orderView.hidden = true;
  confirmView.hidden = false;
}

function setStatus(text, variant) {
  const el = document.getElementById('c-status');
  el.textContent = text;
  el.className = variant ? `pill ${variant}` : 'pill';
}

function renderShipment(shipment) {
  setText('c-carrier', shipment.carrier);
  setText('c-cost', idr.format(shipment.costIdr));
  setText('c-eta', shipment.etaDays === 1 ? '1 day' : `${shipment.etaDays} days`);
  setText('c-shipment-id', shipment.shipmentId);
  document.getElementById('shipment-block').hidden = false;
  setStatus('Shipped', 'done');
}

/**
 * Polls GET /orders/{id} until the async branch produces a shipment.
 *
 * The whole loop is wrapped in ONE span, so every poll lands in a single trace
 * rather than scattering one trace per request. That trace is deliberately
 * separate from the place-order trace: it's the READ path
 * (browser → order-service → [Postgres] + [shipping-service → Postgres]),
 * a structurally different shape worth comparing against the write path.
 */
function awaitShipment(orderId) {
  return tracer.startActiveSpan('await-shipment', async (span) => {
    span.setAttribute('app.order_id', orderId);
    try {
      for (let attempt = 1; attempt <= MAX_POLLS; attempt++) {
        const res = await fetch(`${API_BASE_URL}/orders/${encodeURIComponent(orderId)}`);
        if (res.ok) {
          const body = await res.json();
          if (body.shipment) {
            span.setAttribute('app.polls', attempt);
            span.setAttribute('app.carrier', body.shipment.carrier);
            renderShipment(body.shipment);
            return;
          }
        }
        await sleep(POLL_INTERVAL_MS);
      }
      // Not an error exactly — the async branch just didn't finish in time.
      span.setAttribute('app.timed_out', true);
      setStatus('Shipping still pending', 'pending');
    } catch (err) {
      span.recordException(err);
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(err) });
      setStatus('Shipping status unavailable', '');
    } finally {
      span.end();
    }
  });
}
