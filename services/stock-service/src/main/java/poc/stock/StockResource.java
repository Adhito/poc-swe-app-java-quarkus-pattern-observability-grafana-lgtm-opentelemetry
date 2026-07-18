package poc.stock;

import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

import org.jboss.logging.Logger;

import jakarta.ws.rs.GET;
import jakarta.ws.rs.NotFoundException;
import jakarta.ws.rs.POST;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.PathParam;
import jakarta.ws.rs.WebApplicationException;

@Path("/stock")
public class StockResource {

    private static final Logger LOG = Logger.getLogger(StockResource.class);

    // Stub data for Phase 1 — replaced by PostgreSQL via instrumented JDBC in Phase 2 (PRD §7)
    private final Map<String, StockItem> stock = new ConcurrentHashMap<>(Map.of(
            "SKU-1", new StockItem("SKU-1", "Mechanical keyboard", 42),
            "SKU-2", new StockItem("SKU-2", "USB-C dock", 17),
            "SKU-3", new StockItem("SKU-3", "27-inch monitor", 8),
            "SKU-4", new StockItem("SKU-4", "Laptop stand", 0)));

    @GET
    @Path("/{sku}")
    public StockItem get(@PathParam("sku") String sku) {
        StockItem item = stock.get(sku);
        if (item == null) {
            throw new NotFoundException("unknown sku: " + sku);
        }
        LOG.infof("Stock lookup %s -> %d available", sku, item.quantity());
        return item;
    }

    @POST
    @Path("/reserve")
    public StockItem reserve(ReserveRequest request) {
        if (request == null || request.sku() == null || request.quantity() <= 0) {
            throw new WebApplicationException("sku and a positive quantity are required", 400);
        }
        StockItem updated = stock.computeIfPresent(request.sku(), (sku, item) -> {
            if (item.quantity() < request.quantity()) {
                throw new WebApplicationException("insufficient stock for " + sku, 409);
            }
            return new StockItem(sku, item.name(), item.quantity() - request.quantity());
        });
        if (updated == null) {
            throw new NotFoundException("unknown sku: " + request.sku());
        }
        LOG.infof("Reserved %d x %s -> %d remaining", request.quantity(), request.sku(), updated.quantity());
        return updated;
    }
}
