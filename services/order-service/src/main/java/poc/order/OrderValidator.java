package poc.order;

import io.opentelemetry.instrumentation.annotations.WithSpan;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.ws.rs.WebApplicationException;

/**
 * The manual-instrumentation demo (S4): validate-order shows up as its own
 * span in the trace waterfall. This lives in its own CDI bean because
 * interceptor bindings like @WithSpan do NOT fire on self-invocation — the
 * same annotation on a private method in OrderResource would silently
 * produce no span.
 */
@ApplicationScoped
public class OrderValidator {

    @WithSpan("validate-order")
    public void validate(OrderRequest request) {
        if (request == null || request.sku() == null || request.sku().isBlank() || request.quantity() <= 0) {
            throw new WebApplicationException("sku and a positive quantity are required", 400);
        }
    }
}
