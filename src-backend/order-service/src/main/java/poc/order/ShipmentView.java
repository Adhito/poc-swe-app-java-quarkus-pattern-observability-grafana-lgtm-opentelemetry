package poc.order;

// Shipment as returned by shipping-service (Go) GET /shipments/{orderId}
public record ShipmentView(
        String shipmentId,
        String orderId,
        String carrier,
        double costIdr,
        int etaDays,
        String status) {
}
