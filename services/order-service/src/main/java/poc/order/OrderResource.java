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
        validator.validate(request); // @WithSpan("validate-order") — S4

        StockItem stock;
        try {
            stock = stockClient.getStock(request.sku());
        } catch (WebApplicationException e) {
            if (e.getResponse().getStatus() == 404) {
                throw new WebApplicationException("unknown sku: " + request.sku(), 400);
            }
            throw e;
        }

        if (stock.quantity() < request.quantity()) {
            throw new WebApplicationException("insufficient stock for " + request.sku(), 409);
        }

        String orderId = UUID.randomUUID().toString();
        LOG.infof("Order %s placed: %d x %s", orderId, request.quantity(), request.sku());

        // Emit inside the request span so the producer inherits the trace context
        // (REST->Kafka boundary). Fire-and-forget: the 201 must not block on Kafka,
        // so the consumer span legitimately outlives the HTTP response (PRD 4.4).
        orderEmitter.send(new OrderPlaced(orderId, request.sku(), request.quantity()));

        return Response.status(Response.Status.CREATED)
                .entity(new OrderResponse(orderId, request.sku(), request.quantity(), "PLACED"))
                .build();
    }
}
