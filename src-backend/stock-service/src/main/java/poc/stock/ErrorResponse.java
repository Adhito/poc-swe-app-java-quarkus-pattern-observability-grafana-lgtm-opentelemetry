package poc.stock;

// Matches what the frontend expects: body.error (frontend/src/app.js)
public record ErrorResponse(String error) {
}
