package poc.stock;

// Matches the Phase 2 schema: stock(sku, name, quantity) — PRD 5.5
public record StockItem(String sku, String name, int quantity) {
}
