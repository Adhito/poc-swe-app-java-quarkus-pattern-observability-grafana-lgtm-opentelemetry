package poc.stock;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;

import org.jboss.logging.Logger;

import io.agroal.api.AgroalDataSource;
import jakarta.inject.Inject;
import jakarta.ws.rs.GET;
import jakarta.ws.rs.InternalServerErrorException;
import jakarta.ws.rs.NotFoundException;
import jakarta.ws.rs.POST;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.PathParam;
import jakarta.ws.rs.WebApplicationException;

/**
 * Plain JDBC on purpose (no ORM): the SQL below is exactly what shows up in
 * the db.statement attribute of the JDBC spans (S2), which is the point of
 * this phase. The datasource is telemetry-wrapped via
 * quarkus.datasource.jdbc.telemetry=true.
 */
@Path("/stock")
public class StockResource {

    private static final Logger LOG = Logger.getLogger(StockResource.class);

    @Inject
    AgroalDataSource dataSource;

    @GET
    @Path("/{sku}")
    public StockItem get(@PathParam("sku") String sku) {
        LOG.infof("Stock lookup requested: sku=%s", sku);
        try (Connection conn = dataSource.getConnection();
                PreparedStatement stmt = conn.prepareStatement(
                        "SELECT sku, name, quantity FROM stock WHERE sku = ?")) {
            stmt.setString(1, sku);
            try (ResultSet rs = stmt.executeQuery()) {
                if (!rs.next()) {
                    LOG.warnf("Stock lookup failed: unknown sku=%s", sku);
                    throw new NotFoundException("unknown sku: " + sku);
                }
                StockItem item = new StockItem(rs.getString("sku"), rs.getString("name"), rs.getInt("quantity"));
                LOG.infof("Stock lookup %s -> %d available", sku, item.quantity());
                return item;
            }
        } catch (SQLException e) {
            LOG.errorf(e, "Stock lookup errored: sku=%s", sku);
            throw new InternalServerErrorException("stock lookup failed for " + sku, e);
        }
    }

    @POST
    @Path("/reserve")
    public StockItem reserve(ReserveRequest request) {
        if (request == null || request.sku() == null || request.quantity() <= 0) {
            LOG.warnf("Reserve rejected: invalid request sku=%s qty=%s",
                    request == null ? "<null>" : request.sku(),
                    request == null ? "<null>" : String.valueOf(request.quantity()));
            throw new WebApplicationException("sku and a positive quantity are required", 400);
        }
        LOG.infof("Reserve requested: sku=%s qty=%d", request.sku(), request.quantity());
        try (Connection conn = dataSource.getConnection()) {
            try (PreparedStatement stmt = conn.prepareStatement(
                    "UPDATE stock SET quantity = quantity - ? WHERE sku = ? AND quantity >= ? "
                            + "RETURNING sku, name, quantity")) {
                stmt.setInt(1, request.quantity());
                stmt.setString(2, request.sku());
                stmt.setInt(3, request.quantity());
                try (ResultSet rs = stmt.executeQuery()) {
                    if (rs.next()) {
                        StockItem updated = new StockItem(
                                rs.getString("sku"), rs.getString("name"), rs.getInt("quantity"));
                        LOG.infof("Reserved %d x %s -> %d remaining",
                                request.quantity(), request.sku(), updated.quantity());
                        return updated;
                    }
                }
            }
            // no row updated: either the sku doesn't exist (404) or stock is short (409)
            try (PreparedStatement check = conn.prepareStatement(
                    "SELECT quantity FROM stock WHERE sku = ?")) {
                check.setString(1, request.sku());
                try (ResultSet rs = check.executeQuery()) {
                    if (!rs.next()) {
                        LOG.warnf("Reserve rejected: unknown sku=%s", request.sku());
                        throw new NotFoundException("unknown sku: " + request.sku());
                    }
                    LOG.warnf("Reserve rejected: insufficient stock sku=%s requested=%d available=%d",
                            request.sku(), request.quantity(), rs.getInt("quantity"));
                    throw new WebApplicationException("insufficient stock for " + request.sku(), 409);
                }
            }
        } catch (SQLException e) {
            LOG.errorf(e, "Reserve errored: sku=%s", request.sku());
            throw new InternalServerErrorException("reserve failed for " + request.sku(), e);
        }
    }
}
