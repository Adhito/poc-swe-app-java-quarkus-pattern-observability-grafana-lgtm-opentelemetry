package poc.notification;

import org.eclipse.microprofile.reactive.messaging.Incoming;
import org.eclipse.microprofile.rest.client.inject.RestClient;
import org.jboss.logging.Logger;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;

/**
 * Consumes OrderPlaced from Kafka (PRD 5.3), then continues the async branch
 * into the Go services.
 *
 * Two propagation mechanisms meet here: the Kafka connector's OTel
 * instrumentation extracts the traceparent from the record headers (so this
 * consumer span joins the SAME trace as the order — S5), and the REST client
 * below injects that same context into an HTTP call to shipping-service,
 * which is written in Go. The result is one trace spanning
 * browser → HTTP → SQL → Kafka → Java → Go → Go.
 */
@ApplicationScoped
public class NotificationConsumer {

    private static final Logger LOG = Logger.getLogger(NotificationConsumer.class);

    @Inject
    @RestClient
    ShippingClient shippingClient;

    @Incoming("orders-in")
    public void consume(OrderPlaced order) {
        LOG.infof("Received OrderPlaced from `orders` topic: orderId=%s sku=%s qty=%d",
                order.orderId(), order.sku(), order.quantity());
        LOG.infof("Notification sent for order %s (%d x %s)",
                order.orderId(), order.quantity(), order.sku());

        // Called INSIDE the @Incoming method so the Kafka consumer span is the
        // active parent — the same in-context rule that made the REST->Kafka
        // producer work in Phase 4. Moving this to another thread or an async
        // callback without propagating context would split the trace here.
        try {
            ShipmentResponse shipment = shippingClient.createShipment(
                    new ShipmentRequest(order.orderId(), order.sku(), order.quantity()));
            LOG.infof("Shipment created for order %s: %s via %s (IDR %.0f, ETA %d day(s))",
                    order.orderId(), shipment.shipmentId(), shipment.carrier(),
                    shipment.costIdr(), shipment.etaDays());
        } catch (Exception e) {
            // Deliberately swallowed: letting this propagate would fail the
            // Kafka message and put the consumer into a retry loop over a
            // downstream hiccup. The failure is still visible as an error on
            // the span and in the log line below.
            LOG.errorf(e, "Shipment creation failed for order %s", order.orderId());
        }
    }
}
