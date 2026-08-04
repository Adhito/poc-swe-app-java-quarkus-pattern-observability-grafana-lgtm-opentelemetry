package poc.order;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.UUID;

import org.eclipse.microprofile.reactive.messaging.Channel;
import org.eclipse.microprofile.reactive.messaging.Emitter;
import org.eclipse.microprofile.rest.client.inject.RestClient;
import org.jboss.logging.Logger;

import io.agroal.api.AgroalDataSource;
import io.opentelemetry.api.trace.Span;
import jakarta.inject.Inject;
import jakarta.ws.rs.GET;
import jakarta.ws.rs.InternalServerErrorException;
import jakarta.ws.rs.NotFoundException;
import jakarta.ws.rs.POST;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.PathParam;
import jakarta.ws.rs.WebApplicationException;
import jakarta.ws.rs.core.Response;

@Path("/orders")
public class OrderResource {

    private static final Logger LOG = Logger.getLogger(OrderResource.class);

    @RestClient
    StockClient stockClient;

    @RestClient
    ShippingClient shippingClient;

    @Inject
    OrderValidator validator;

    @Inject
    AgroalDataSource dataSource;

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

        // Store the trace id alongside the order so the confirmation page can
        // deep-link straight to this order's trace in Grafana.
        String traceId = Span.current().getSpanContext().getTraceId();
        insertOrder(orderId, request.sku(), request.quantity(), traceId);

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

    /**
     * Read path for the confirmation page.
     *
     * Reads the order this service owns, then asks shipping-service (Go) for
     * the shipment it owns. The shipment is null until the async branch
     * completes — the page polls this endpoint until it appears, which is what
     * makes the post-response async work visible in the UI.
     */
    @GET
    @Path("/{orderId}")
    public OrderStatusResponse getOrder(@PathParam("orderId") String orderId) {
        LOG.infof("Order status requested: orderId=%s", orderId);

        OrderStatusResponse order = selectOrder(orderId);
        if (order == null) {
            LOG.warnf("Order status failed: unknown orderId=%s", orderId);
            throw new NotFoundException("unknown order: " + orderId);
        }

        ShipmentView shipment = null;
        try {
            shipment = shippingClient.getByOrderId(orderId);
        } catch (WebApplicationException e) {
            if (e.getResponse().getStatus() == 404) {
                // Expected while the async branch is still in flight.
                LOG.infof("No shipment yet for order %s (async branch still in flight)", orderId);
            } else {
                // Don't fail the whole read over a shipping hiccup — the page
                // can still show the order itself.
                LOG.errorf(e, "Shipment lookup failed for order %s", orderId);
            }
        }

        return new OrderStatusResponse(
                order.orderId(), order.sku(), order.quantity(),
                shipment != null ? "SHIPPED" : order.status(),
                order.traceId(), shipment);
    }

    private void insertOrder(String orderId, String sku, int quantity, String traceId) {
        try (Connection conn = dataSource.getConnection();
                PreparedStatement stmt = conn.prepareStatement(
                        "INSERT INTO orders (order_id, sku, quantity, status, trace_id) "
                                + "VALUES (?, ?, ?, ?, ?)")) {
            stmt.setString(1, orderId);
            stmt.setString(2, sku);
            stmt.setInt(3, quantity);
            stmt.setString(4, "PLACED");
            stmt.setString(5, traceId);
            stmt.executeUpdate();
        } catch (SQLException e) {
            LOG.errorf(e, "Failed to persist order %s", orderId);
            throw new InternalServerErrorException("failed to persist order " + orderId, e);
        }
    }

    /** Returns the stored order, or null if there's no such row. */
    private OrderStatusResponse selectOrder(String orderId) {
        try (Connection conn = dataSource.getConnection();
                PreparedStatement stmt = conn.prepareStatement(
                        "SELECT order_id, sku, quantity, status, trace_id FROM orders WHERE order_id = ?")) {
            stmt.setString(1, orderId);
            try (ResultSet rs = stmt.executeQuery()) {
                if (!rs.next()) {
                    return null;
                }
                return new OrderStatusResponse(
                        rs.getString("order_id"),
                        rs.getString("sku"),
                        rs.getInt("quantity"),
                        rs.getString("status"),
                        rs.getString("trace_id"),
                        null);
            }
        } catch (SQLException e) {
            LOG.errorf(e, "Order lookup errored: orderId=%s", orderId);
            throw new InternalServerErrorException("order lookup failed for " + orderId, e);
        }
    }
}
