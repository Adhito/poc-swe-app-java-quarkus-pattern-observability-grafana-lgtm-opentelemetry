package poc.order;

import org.eclipse.microprofile.rest.client.inject.RegisterRestClient;

import jakarta.ws.rs.GET;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.PathParam;

/**
 * MicroProfile REST client for stock-service. The OTel extension instruments
 * this client automatically, so the W3C traceparent header propagates to
 * stock-service without any manual code (PRD 6.1).
 */
@RegisterRestClient(configKey = "stock")
@Path("/stock")
public interface StockClient {

    @GET
    @Path("/{sku}")
    StockItem getStock(@PathParam("sku") String sku);
}
