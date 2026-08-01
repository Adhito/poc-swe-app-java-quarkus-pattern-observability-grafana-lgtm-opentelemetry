package poc.notification;

// Mirrors order-service's OrderPlaced — same JSON shape on the `orders` topic.
public record OrderPlaced(String orderId, String sku, int quantity) {
}
