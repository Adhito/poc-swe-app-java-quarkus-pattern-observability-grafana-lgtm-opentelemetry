package poc.order;

/**
 * What GET /orders/{orderId} returns. `shipment` is null until the async
 * branch (Kafka → notification-service → shipping-service → carrier-service)
 * has finished — which is precisely the window the confirmation page polls
 * through.
 */
public record OrderStatusResponse(
        String orderId,
        String sku,
        int quantity,
        String status,
        String traceId,
        ShipmentView shipment) {
}
