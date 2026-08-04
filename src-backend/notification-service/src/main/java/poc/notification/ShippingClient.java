package poc.notification;

import org.eclipse.microprofile.rest.client.inject.RegisterRestClient;

import jakarta.ws.rs.Consumes;
import jakarta.ws.rs.POST;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.core.MediaType;

/**
 * MicroProfile REST client for shipping-service — which is written in Go.
 *
 * The OTel extension instruments this client automatically, so the W3C
 * traceparent header crosses the language boundary with no manual code on
 * this side. The Go service has to opt into the same W3C propagators
 * explicitly (see its otel.go); that asymmetry is the point of the exercise.
 */
@RegisterRestClient(configKey = "shipping")
@Path("/shipments")
public interface ShippingClient {

    @POST
    @Consumes(MediaType.APPLICATION_JSON)
    ShipmentResponse createShipment(ShipmentRequest request);
}
