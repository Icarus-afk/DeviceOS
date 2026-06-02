# Troubleshooting & FAQ

## Common Issues

### Database is locked / SQLITE_BUSY errors

SQLite is a single-writer database. Under heavy concurrent write load, you may see `database is locked` errors.

**Solutions:**
- Increase `max_open_conns` in config (default 25, max 100)
- Reduce concurrent write connections
- Ensure WAL mode is enabled (`journal_mode=WAL` is set by default)
- Check `busy_timeout=5000` is in effect (5 second wait before timeout)

### Telemetry is not appearing in queries

**Check:**
1. Is `telemetry_ttl` set? If so, data older than the TTL is automatically pruned.
2. Is the device registered? Unregistered devices cannot ingest telemetry.
3. Are you using the correct `X-Org-ID`? Telemetry is scoped to the org that owns the device.
4. Check the server logs for ingestion errors: `slog` output shows the request path, status, and duration.

### Cannot connect to MQTT broker

**Check:**
1. Is the MQTT module enabled? It is enabled by default in `cmd/deviceos/start.go`.
2. Is port 1883 accessible? The MQTT broker listens on `modules.mqtt.port` (default 1883).
3. Does the device exist in DeviceOS? MQTT authentication validates the device ID and secret via the device's API key.
4. Check server logs: MQTT connection attempts are logged at `info` level.

### JWT token is rejected

**Causes:**
- Token is expired (default expiration: 24 hours for users, unlimited for devices)
- Wrong secret: `modules.jwt_secret` must match the secret used to sign the token
- Missing or malformed `Authorization: Bearer <token>` header
- The token was issued for a different org or device

### WebSocket connection drops

**Causes:**
- Rate limiting: check `rate_limit_rpm` config. WebSocket upgrades count toward the rate limit.
- Proxy timeout: if behind nginx, ensure `proxy_read_timeout` is high enough (default 60s).
- Ping/pong: the server sends pings every 30 seconds. If the client does not respond within 10 seconds, the connection is closed.

### `deviceos backup` fails with "database is locked"

The backup command opens the database in read-only mode, but if the server is writing concurrently, the backup may fail. Run backup during low-write periods or stop the server first.

### Getting "too many open files"

DeviceOS opens one connection per WebSocket client and per MQTT client. With thousands of connected clients, you may hit the OS file descriptor limit.

**Fix:**
```bash
# Check current limit
ulimit -n

# Increase system-wide limit
echo "fs.file-max = 100000" >> /etc/sysctl.conf
sysctl -p

# Increase process limit (in systemd service unit)
LimitNOFILE=65536
```

## FAQ

### Can I use PostgreSQL or MySQL instead of SQLite?

No. DeviceOS uses SQLite as its single embedded database. The database abstraction layer (`internal/db`) wraps `database/sql`, but the migration system, pragmas, and query patterns are SQLite-specific. A future version may support alternative backends.

### How many devices can DeviceOS handle?

DeviceOS is designed for IoT fleet sizes in the thousands to tens of thousands of devices. SQLite's single-writer constraint means that extremely high-throughput write workloads (millions of devices) would benefit from a different architecture.

For reference, the embedded MQTT broker (mochi-mqtt) handles thousands of concurrent connections on modest hardware.

### Can I run DeviceOS behind a load balancer?

Yes, but all instances share the same SQLite database file, so horizontal scaling is limited by SQLite's single-writer constraint. For multi-instance deployments, place DeviceOS behind a reverse proxy (nginx, Caddy, Traefik) and use sticky sessions if needed for WebSocket connections.

### Is there a Web UI?

Yes. The dashboard module serves a single-page application at `http://localhost:8080/` (or the configured host/port). It provides device management, telemetry visualization, OTA deployment management, and alert configuration.

### How do I reset the admin API key?

If you lose the admin API key:
1. Stop DeviceOS
2. Delete the `api_keys` table row or set a new key in config (`modules.admin_api_key`)
3. Restart

### Can I contribute a new module?

Yes. See [Docs/development.md](development.md) for the module interface specification and conventions. All modules live in `modules/` and implement the `registry.Module` interface.
