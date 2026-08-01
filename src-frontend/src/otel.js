// zone.js gives ZoneContextManager a way to keep the active span/context alive
// across the async `await fetch(...)`, so the auto fetch span nests under our
// manual root span instead of becoming its own root.
import 'zone.js';

import { trace } from '@opentelemetry/api';
import { WebTracerProvider, BatchSpanProcessor } from '@opentelemetry/sdk-trace-web';
import { resourceFromAttributes } from '@opentelemetry/resources';
import { ZoneContextManager } from '@opentelemetry/context-zone';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import { registerInstrumentations } from '@opentelemetry/instrumentation';
import { FetchInstrumentation } from '@opentelemetry/instrumentation-fetch';

// Runtime config injected by the container entrypoint (config.js), so one image
// works local + EKS (PRD Stage B: identical images, config via env).
const cfg = window.__APP_CONFIG__ || {};
const OTLP_URL = cfg.otlpUrl || 'http://localhost:4318/v1/traces';
const API_BASE_URL = cfg.apiBaseUrl || 'http://localhost:8080';

// build a RegExp that matches a literal URL substring
const urlRe = (u) => new RegExp(u.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));

const provider = new WebTracerProvider({
  resource: resourceFromAttributes({
    'service.name': 'frontend',
    'service.version': '1.0.0',
  }),
  // OTLP/HTTP export to the collector's external ingress (out-of-band, PRD 4.2)
  spanProcessors: [new BatchSpanProcessor(new OTLPTraceExporter({ url: OTLP_URL }))],
});

// Default propagator is W3C Trace Context + Baggage — exactly what we want (PRD 6.1)
provider.register({ contextManager: new ZoneContextManager() });

registerInstrumentations({
  instrumentations: [
    new FetchInstrumentation({
      // THE critical line: the SDK only injects `traceparent` into cross-origin
      // fetches whose URL matches this list. Without it the browser trace and
      // the backend trace get different IDs (the classic split-trace bug).
      propagateTraceHeaderCorsUrls: [urlRe(API_BASE_URL)],
      // don't instrument the exporter's own POSTs to the collector — that would
      // generate spans about sending spans (a telemetry feedback loop)
      ignoreUrls: [urlRe(OTLP_URL)],
    }),
  ],
});

export const tracer = trace.getTracer('frontend', '1.0.0');
export { API_BASE_URL };
