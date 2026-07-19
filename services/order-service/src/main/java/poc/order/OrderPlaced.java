package poc.order;

// Event published to the `orders` topic after a successful order (S5).
public record OrderPlaced(String orderId, String sku, int quantity) {
}
