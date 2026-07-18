package poc.order;

import java.util.UUID;

import org.eclipse.microprofile.rest.client.inject.RestClient;
import org.jboss.logging.Logger;

import jakarta.ws.rs.POST;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.WebApplicationException;
import jakarta.ws.rs.core.Response;

@Path("/orders")
public class OrderResource {

    private static final Logger LOG = Logger.getLogger(OrderResource.class);

    @RestClient
    StockClient stockClient;

    @POST
    public Response placeOrder(OrderRequest request) {
        if (request == null || request.sku() == null || request.sku().isBlank() || request.quantity() <= 0) {
            throw new WebApplicationException("sku and a positive quantity are required", 400);
        }

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
        return Response.status(Response.Status.CREATED)
                .entity(new OrderResponse(orderId, request.sku(), request.quantity(), "PLACED"))
                .build();
    }
}
