package poc.order;

import java.util.UUID;

import org.eclipse.microprofile.reactive.messaging.Channel;
import org.eclipse.microprofile.reactive.messaging.Emitter;
import org.eclipse.microprofile.rest.client.inject.RestClient;
import org.jboss.logging.Logger;

import jakarta.inject.Inject;
import jakarta.ws.rs.POST;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.WebApplicationException;
import jakarta.ws.rs.core.Response;

@Path("/orders")
public class OrderResource {

    private static final Logger LOG = Logger.getLogger(OrderResource.class);

    @RestClient
    StockClient stockClient;

    @Inject
    OrderValidator validator;

    // async producer to the `orders` topic; the OTel Kafka instrumentation injects
    // the current trace context into the record headers (S5)
    @Channel("orders-out")
    Emitter<OrderPlaced> orderEmitter;

    @POST
    public Response placeOrder(OrderRequest request) {
        LOG.infof("Order request received: sku=%s qty=%s",
                request == null ? "<null>" : request.sku(),
                request == null ? "<null>" : String.valueOf(request.quantity()));

        validator.validate(request); // @WithSpan("validate-order") — S4

        StockItem stock;
        try {
            stock = stockClient.getStock(request.sku());
        } catch (WebApplicationException e) {
            if (e.getResponse().getStatus() == 404) {
                LOG.warnf("Order rejected: unknown sku=%s (stock-service returned 404)", request.sku());
                throw new WebApplicationException("unknown sku: " + request.sku(), 400);
            }
            LOG.errorf(e, "Stock lookup failed for sku=%s", request.sku());
            throw e;
        }

        if (stock.quantity() < request.quantity()) {
            LOG.warnf("Order rejected: insufficient stock sku=%s requested=%d available=%d",
                    request.sku(), request.quantity(), stock.quantity());
            throw new WebApplicationException("insufficient stock for " + request.sku(), 409);
        }
        LOG.infof("Stock check passed: sku=%s requested=%d available=%d",
                request.sku(), request.quantity(), stock.quantity());

        String orderId = UUID.randomUUID().toString();
        LOG.infof("Order %s placed: %d x %s", orderId, request.quantity(), request.sku());

        // Emit inside the request span so the producer inherits the trace context
        // (REST->Kafka boundary). Fire-and-forget: the 201 must not block on Kafka,
        // so the consumer span legitimately outlives the HTTP response (PRD 4.4).
        orderEmitter.send(new OrderPlaced(orderId, request.sku(), request.quantity()));
        LOG.infof("Published OrderPlaced to `orders` topic: orderId=%s", orderId);

        return Response.status(Response.Status.CREATED)
                .entity(new OrderResponse(orderId, request.sku(), request.quantity(), "PLACED"))
                .build();
    }
}
