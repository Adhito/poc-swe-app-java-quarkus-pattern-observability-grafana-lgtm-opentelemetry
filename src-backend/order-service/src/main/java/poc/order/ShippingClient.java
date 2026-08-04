package poc.order;

import org.eclipse.microprofile.rest.client.inject.RegisterRestClient;

import jakarta.ws.rs.GET;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.PathParam;

/**
 * Read-path client to shipping-service (Go).
 *
 * This puts a Java→Go hop on the READ path too, not just the async write path
 * — so the confirmation page's polling GET produces its own multi-service,
 * cross-language trace, structurally different from the order trace.
 */
@RegisterRestClient(configKey = "shipping")
@Path("/shipments")
public interface ShippingClient {

    @GET
    @Path("/{orderId}")
    ShipmentView getByOrderId(@PathParam("orderId") String orderId);
}
