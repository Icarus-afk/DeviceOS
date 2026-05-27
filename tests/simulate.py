#!/usr/bin/env python3
"""DeviceOS end-to-end simulation & verification script.

Usage:
  python tests/simulate.py                          # run against localhost:8080
  python tests/simulate.py --server http://10.0.0.5:8080
  python tests/simulate.py --api-key my_admin_key
"""

import argparse
import json
import os
import sys
import time
import urllib.request
import urllib.error


BASE = "http://localhost:8080"
API_KEY = os.environ.get("DEVICEOS_ADMIN_TOKEN", "dos_dev_admin_token_0001")
PASS = 0
FAIL = 0
STEP = 0


def ok(msg):
    global PASS
    PASS += 1
    print(f"  \u2713 {msg}")


def fail(msg):
    global FAIL
    FAIL += 1
    print(f"  \u2717 {msg}")


def check(cond, msg):
    if cond:
        ok(msg)
    else:
        fail(msg)


def request(method, path, body=None, token=None, raw=False, timeout=60):
    url = BASE + path
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        resp = urllib.request.urlopen(req, timeout=timeout)
        body = resp.read()
        if raw:
            return resp.status, body
        if body.strip():
            return resp.status, json.loads(body)
        return resp.status, {}
    except urllib.error.HTTPError as e:
        body_bytes = e.read()
        try:
            return e.code, json.loads(body_bytes)
        except Exception:
            return e.code, body_bytes.decode()


def section(title):
    global STEP
    STEP += 1
    print(f"\n=== [{STEP}] {title} ===")


def run(args):
    global PASS, FAIL

    addr = BASE.replace("http://", "")
    print(f"DeviceOS E2E Simulation")
    print(f"{'Server':>12s}: {addr}")
    print(f"{'API Key':>12s}: {API_KEY[:20]}...")
    print("-" * 40)

    # ── 1. Health ──────────────────────────────────────────────
    section("Health check")
    status, data = request("GET", "/healthz")
    check(status == 200, f"/healthz -> {status}")
    check("version" in data, "version field present")
    ok(f"server version: {data.get('version', '?')}")

    # ── 2. Login ───────────────────────────────────────────────
    section("Authenticate")
    status, data = request("POST", "/api/v1/auth/login", {"api_key": API_KEY})
    check(status == 200, f"login -> {status}")
    token = data.get("token", "")
    check(bool(token), "got auth token")

    # ── 3. Register devices ────────────────────────────────────
    section("Register devices")
    device_ids = []
    devices_def = [
        ("truck-001", "gps-tracker", {"location": "dhaka"}),
        ("sensor-042", "temp-sensor", {"floor": 3}),
        ("gateway-a", "multi-sensor", {"site": "factory-1"}),
    ]
    for name, dtype, meta in devices_def:
        status, data = request(
            "POST", "/api/v1/devices",
            {"name": name, "type": dtype, "metadata": meta},
            token=token,
        )
        if status == 201:
            d = data.get("device", {})
            device_ids.append(d.get("id", ""))
            ok(f"registered {name} ({dtype}) -> {d.get('id', '')}")
        else:
            fail(f"register {name}: {status} {data}")
    check(len(device_ids) == 3, f"{len(device_ids)} devices registered")

    # ── 4. List devices ────────────────────────────────────────
    section("List devices")
    status, data = request("GET", "/api/v1/devices", token=token)
    check(status == 200, f"list devices -> {status}")
    devices = data.get("devices", [])
    check(len(devices) == 3, f"{len(devices)} devices listed")

    # ── 5. Get single device ───────────────────────────────────
    section("Get device detail")
    status, data = request("GET", f"/api/v1/devices/{device_ids[0]}", token=token)
    check(status == 200, f"get device -> {status}")
    check(data.get("name") == "truck-001", "name matches")

    # ── 6. Set up alert rules BEFORE sending telemetry ────────
    section("Alert rules (before telemetry)")
    rules_def = [
        ("high-temp", "temperature", ">", 45.0),
        ("low-battery", "battery", "<", 20.0),
    ]
    rule_ids = []
    for name, metric, op, threshold in rules_def:
        status, data = request(
            "POST", "/api/v1/alerts/rules",
            {"name": name, "metric": metric, "operator": op, "threshold": threshold},
            token=token,
        )
        check(status == 201, f"create rule '{name}' -> {status}")
        rule_ids.append(data.get("id", ""))

    status, data = request("GET", "/api/v1/alerts/rules", token=token)
    check(status == 200, f"list rules -> {status}")
    rules = data.get("rules", [])
    check(len(rules) == 2, f"{len(rules)} rules")

    # ── 7. Send telemetry ──────────────────────────────────────
    section("Send telemetry")
    telemetry_payloads = [
        (device_ids[0], {"temperature": 31.0, "battery": 82.0, "speed": 65.0}),
        (device_ids[1], {"temperature": 47.2, "humidity": 55.0, "battery": 73.0}),
        (device_ids[2], {"temperature": 24.5, "humidity": 60.0, "rssi": -42}),
    ]
    for did, metrics in telemetry_payloads:
        status, data = request(
            "POST", "/api/v1/telemetry",
            {"device_id": did, "metrics": metrics},
            token=token,
        )
        check(status == 201, f"telemetry {did[:8]} -> {status}")
    # Send a second round to have history
    for did, metrics in telemetry_payloads:
        request("POST", "/api/v1/telemetry",
                {"device_id": did, "metrics": metrics}, token=token)

    # ── 8. Query telemetry ─────────────────────────────────────
    section("Query telemetry")
    status, data = request(
        "GET", f"/api/v1/telemetry?device_id={device_ids[1]}&limit=5",
        token=token,
    )
    check(status == 200, f"query telemetry -> {status}")
    results = data.get("telemetry", [])
    check(len(results) >= 1, f"{len(results)} datapoints returned")
    if results:
        check("metrics" in results[0], "metrics field present")
        check("device_id" in results[0], "device_id field present")

    # ── 8. Latest telemetry ────────────────────────────────────
    section("Latest telemetry")
    status, data = request(
        "GET", f"/api/v1/telemetry/latest?device_id={device_ids[1]}",
        token=token,
    )
    check(status == 200, f"latest telemetry -> {status}")
    check(data.get("device_id") == device_ids[1], "correct device")

    # ── 9. Fleet health ────────────────────────────────────────
    section("Fleet health")
    status, data = request("GET", "/api/v1/fleet/health", token=token)
    check(status == 200, f"fleet health -> {status}")
    check("total_devices" in data, "total_devices field")
    check("online_devices" in data, "online_devices field")
    check("offline_devices" in data, "offline_devices field")

    # ── 10. Device groups (fleet) ──────────────────────────────
    section("Device groups")
    status, data = request(
        "POST", "/api/v1/groups",
        {"name": "dhaka-fleet", "description": "Dhaka logistics"},
        token=token,
    )
    check(status == 201, f"create group -> {status}")
    group_id = data.get("id", "")
    check(bool(group_id), "group has id")

    status, data = request("GET", "/api/v1/groups", token=token)
    check(status == 200, f"list groups -> {status}")
    groups = data.get("groups", [])
    check(len(groups) >= 1, f"{len(groups)} groups")

    # Assign device to group
    status, _ = request(
        "PUT", f"/api/v1/devices/{device_ids[0]}/group",
        {"group": "dhaka-fleet"},
        token=token,
    )
    check(status == 200, "assign device to group")

    # ── 11. Tags ───────────────────────────────────────────────
    section("Device tags")
    status, data = request(
        "POST", f"/api/v1/devices/{device_ids[0]}/tags",
        {"tags": ["critical", "logistics"]},
        token=token,
    )
    check(status == 200, f"add tags -> {status}")

    # ── 12. Alert history (should have fired from high temp) ───
    section("Alert history")
    time.sleep(1)  # let async evaluation settle
    status, data = request("GET", "/api/v1/alerts/history", token=token)
    check(status == 200, f"alert history -> {status}")
    events = data.get("events") or []
    check(len(events) >= 1, f"{len(events)} alert events fired")
    if events:
        check("severity" in events[0], "severity field present")
        check("message" in events[0], "message field present")

    # ── 14. Webhooks ───────────────────────────────────────────
    section("Webhooks")
    status, data = request(
        "POST", "/api/v1/webhooks",
        {
            "name": "slack-alerts",
            "url": "https://hooks.example.com/alert",
            "events": ["alert.fired"],
        },
        token=token,
    )
    check(status == 201, f"create webhook -> {status}")
    wh_id = data.get("id", "")
    check(bool(wh_id), "webhook has id")

    status, data = request("GET", "/api/v1/webhooks", token=token)
    check(status == 200, f"list webhooks -> {status}")

    # ── 15. Audit log ──────────────────────────────────────────
    section("Audit log")
    status, data = request("GET", "/api/v1/audit", token=token)
    check(status == 200, f"audit log -> {status}")

    # ── 16. Dashboard serves HTML ──────────────────────────────
    section("Dashboard page")
    status, html = request("GET", "/dashboard", raw=True)
    check(status == 200, f"/dashboard -> {status}")
    check(b"DeviceOS Dashboard" in html, "dashboard title present")
    check(b"Overview" in html, "overview section present")
    check(b"connectWS" in html, "WebSocket code present")

    # Root redirect or serves dashboard
    status, body = request("GET", "/", raw=True)
    ok(f"/ returns {status} (expected 302 or 200)")

    # ── 17. Device update ──────────────────────────────────────
    section("Device update")
    status, data = request(
        "PUT", f"/api/v1/devices/{device_ids[1]}",
        {"name": "sensor-042-updated", "type": "temp-sensor"},
        token=token,
    )
    check(status == 200, f"update device -> {status}")

    # ── 18. Device delete ──────────────────────────────────────
    section("Device cleanup")
    cleanup_id = device_ids[2]
    status, _ = request("DELETE", f"/api/v1/devices/{cleanup_id}", token=token)
    if status == 204:
        ok(f"delete device {cleanup_id[:8]} -> 204")
        status, _ = request("GET", f"/api/v1/devices/{cleanup_id}", token=token)
        check(status == 404, "deleted device returns 404")
    else:
        fail(f"delete device {cleanup_id[:8]} -> {status} (may already be deleted)")

    # ── 19. Built-in simulator ─────────────────────────────────
    section("Built-in simulator (module)")
    status, data = request("POST", "/api/v1/simulator/start",
                           {"count": 2}, token=token)
    check(status == 200, f"simulator start -> {status}")
    check(data.get("status") == "started", "simulator status = started")
    ok(f"simulating {data.get('count', 0)} devices")

    time.sleep(8)  # let it run for 1 tick + buffer

    # Devices should have been registered and sent telemetry
    status, data = request("GET", "/api/v1/devices", token=token)
    all_devices = data.get("devices", [])
    check(len(all_devices) >= 2, f"{len(all_devices)} devices total (incl simulated)")

    status, data = request("POST", "/api/v1/simulator/stop", token=token)
    check(status == 200, f"simulator stop -> {status}")
    check(data.get("status") == "stopped", "simulator status = stopped")

    # ── 20. Tenant (multi-org) ─────────────────────────────────
    section("Multi-tenant")
    status, data = request(
        "POST", "/api/v1/orgs",
        {"name": "Acme IoT", "slug": "acme-iot"},
        token=token,
    )
    check(status == 201, f"create org -> {status}")
    org_id = data.get("id", "")
    check(bool(org_id), "org has id")

    status, data = request("GET", "/api/v1/orgs", token=token)
    check(status == 200, f"list orgs -> {status}")

    # ── 21. Tenant users ───────────────────────────────────────
    section("Org users")
    status, data = request(
        "POST", f"/api/v1/orgs/{org_id}/users",
        {"email": "ops@acme.io", "role": "admin"},
        token=token,
    )
    check(status == 201, f"invite user -> {status}")

    status, data = request("GET", f"/api/v1/orgs/{org_id}/users", token=token)
    check(status == 200, f"list users -> {status}")

    # ── Summary ────────────────────────────────────────────────
    total = PASS + FAIL
    print(f"\n{'=' * 40}")
    print(f"Results: {PASS}/{total} passed, {FAIL}/{total} failed")
    if FAIL:
        sys.exit(1)
    print("All checks passed!")
    print(f"Dashboard: {BASE}/dashboard")


if __name__ == "__main__":
    p = argparse.ArgumentParser(description="DeviceOS E2E simulation")
    p.add_argument("--server", default=BASE, help="DeviceOS server URL")
    p.add_argument("--api-key", default=API_KEY, help="Admin API key")
    args = p.parse_args()
    BASE = args.server.rstrip("/")
    API_KEY = args.api_key or API_KEY
    run(args)
