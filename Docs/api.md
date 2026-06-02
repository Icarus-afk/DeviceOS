# DeviceOS API Quickstart

All requests return `application/json`. Authenticate with `Authorization: Bearer <jwt>` or `Authorization: ApiKey <key>`.

## Workflows

### 1. Register an Organization

```bash
curl -X POST http://localhost:8080/api/v1/orgs \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"name": "Acme Corp"}'
```

### 2. Register a Device

```bash
curl -X POST http://localhost:8080/api/v1/devices \
  -H "Authorization: Bearer <jwt>" \
  -H "X-Org-ID: <org-id>" \
  -H "Content-Type: application/json" \
  -d '{"name": "sensor-001", "type": "temp-sensor"}'
```

### 3. Ingest Telemetry (HTTP)

```bash
curl -X POST http://localhost:8080/api/v1/telemetry \
  -H "Authorization: Bearer <device-token>" \
  -H "Content-Type: application/json" \
  -d '{"device_id": "sensor-001", "temperature": 23.5, "humidity": 60}'
```

### 4. Ingest Telemetry (MQTT)

```bash
mosquitto_pub -h localhost -p 1883 \
  -u "sensor-001" -P "<device-secret>" \
  -t "deviceos/sensor-001/telemetry" \
  -m '{"temperature": 23.5, "humidity": 60}'
```

### 5. Ingest Telemetry (WebSocket)

```bash
# Connect to WebSocket telemetry stream
wscat -c "ws://localhost:8080/api/v1/ws/telemetry?token=<device-token>"

# Send telemetry as JSON messages
> {"device_id": "sensor-001", "temperature": 23.5}
```

### 6. Query Recent Telemetry

```bash
curl "http://localhost:8080/api/v1/telemetry?device_id=sensor-001&limit=10" \
  -H "Authorization: Bearer <jwt>"
```

### 7. Create an Alert Rule

```bash
curl -X POST http://localhost:8080/api/v1/alerts/rules \
  -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "High Temperature",
    "metric": "temperature",
    "condition": "gt",
    "threshold": 30.0,
    "device_id": "sensor-001",
    "enabled": true
  }'
```

### 8. Subscribe to Real-Time Events

```bash
wscat -c "ws://localhost:8080/api/v1/ws/events?token=<jwt>"
# Filter by event type:
wscat -c "ws://localhost:8080/api/v1/ws/events?events=telemetry,alert&token=<jwt>"
```

### 9. Send a Command to a Device

```bash
curl -X POST http://localhost:8080/api/v1/devices/sensor-001/commands \
  -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{"type": "reboot", "payload": {"delay_ms": 5000}}'
```

### 10. Deploy OTA Firmware

```bash
# Upload firmware binary
curl -X POST http://localhost:8080/api/v1/firmware \
  -H "Authorization: Bearer <jwt>" \
  -F "file=@firmware.bin" \
  -F "version=1.2.0" \
  -F "device_type=temp-sensor"

# Deploy to a device group
curl -X POST http://localhost:8080/api/v1/firmware/deploy \
  -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{"firmware_id": "<id>", "group_id": "<group-id>"}'
```

### 11. Check Server Health

```bash
curl http://localhost:8080/healthz
# {"status":"ok","version":"0.1.0 Hummingbird","uptime":"5m30s","modules":{...}}
```

## Authentication

### Get a JWT Token

```bash
# Admin login
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"api_key": "your-admin-key"}'
```

## Error Format

All errors follow a consistent JSON structure:

```json
{
  "error": {
    "code": "bad_request|not_found|internal|unauthorized|forbidden|conflict",
    "message": "Human-readable description"
  }
}
```

## Postman / Insomnia

Import `Docs/openapi.yaml` into Postman or Insomnia for a complete, browsable API reference with all 50+ endpoints, request schemas, and response examples.
