package poc.order;

import org.jboss.logging.Logger;

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

    private static final Logger LOG = Logger.getLogger(OrderValidator.class);

    @WithSpan("validate-order")
    public void validate(OrderRequest request) {
        if (request == null || request.sku() == null || request.sku().isBlank() || request.quantity() <= 0) {
            LOG.warnf("Validation failed: sku=%s qty=%s (sku required, qty must be > 0)",
                    request == null ? "<null>" : request.sku(),
                    request == null ? "<null>" : String.valueOf(request.quantity()));
            throw new WebApplicationException("sku and a positive quantity are required", 400);
        }
        LOG.infof("Validation passed: sku=%s qty=%d", request.sku(), request.quantity());
    }
}
