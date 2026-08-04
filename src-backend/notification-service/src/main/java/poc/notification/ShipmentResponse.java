package poc.notification;

// Response from shipping-service (Go) POST /shipments
public record ShipmentResponse(
        String shipmentId,
        String orderId,
        String carrier,
        double costIdr,
        int etaDays,
        String status) {
}
