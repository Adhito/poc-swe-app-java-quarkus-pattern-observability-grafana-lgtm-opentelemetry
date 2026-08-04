package poc.notification;

// Request body for shipping-service (Go) POST /shipments
public record ShipmentRequest(String orderId, String sku, int quantity) {
}
