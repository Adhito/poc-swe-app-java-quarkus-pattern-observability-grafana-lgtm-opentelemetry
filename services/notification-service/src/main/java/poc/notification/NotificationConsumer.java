package poc.notification;

import org.eclipse.microprofile.reactive.messaging.Incoming;
import org.jboss.logging.Logger;

import jakarta.enterprise.context.ApplicationScoped;

/**
 * Consumes OrderPlaced from Kafka (PRD 5.3). Exists purely to demonstrate async
 * trace-context propagation: the Kafka connector's OTel instrumentation extracts
 * the traceparent from the record headers, so this consumer span joins the SAME
 * trace as the order — and the log line below carries that traceId (S5 + S3).
 */
@ApplicationScoped
public class NotificationConsumer {

    private static final Logger LOG = Logger.getLogger(NotificationConsumer.class);

    @Incoming("orders-in")
    public void consume(OrderPlaced order) {
        LOG.infof("Notification sent for order %s (%d x %s)",
                order.orderId(), order.quantity(), order.sku());
    }
}
