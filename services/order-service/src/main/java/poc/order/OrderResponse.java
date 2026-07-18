package poc.order;

public record OrderResponse(String orderId, String sku, int quantity, String status) {
}
