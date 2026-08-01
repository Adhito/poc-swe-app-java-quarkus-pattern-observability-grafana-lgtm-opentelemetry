package poc.stock;

import jakarta.ws.rs.WebApplicationException;
import jakarta.ws.rs.core.Response;
import jakarta.ws.rs.ext.ExceptionMapper;
import jakarta.ws.rs.ext.Provider;

/**
 * The 2-arg WebApplicationException(message, status) constructor (and
 * NotFoundException(message), which extends it) only sets the Java-side
 * exception message (for logs) — the HTTP response body is empty, which the
 * caller's res.json() then fails to parse. This wraps every bare
 * WebApplicationException into {"error": "..."} so the message actually
 * reaches the client, without touching any of the existing throw sites.
 */
@Provider
public class WebApplicationExceptionMapper implements ExceptionMapper<WebApplicationException> {

    @Override
    public Response toResponse(WebApplicationException exception) {
        Response response = exception.getResponse();
        if (response.getEntity() != null) {
            return response; // already carries a body — don't clobber it
        }
        return Response.status(response.getStatus())
                .entity(new ErrorResponse(exception.getMessage()))
                .build();
    }
}
