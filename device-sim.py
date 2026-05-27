#!/usr/bin/env python3
"""DeviceOS — Virtual IoT Device Simulator

Connects to a DeviceOS server as if it were a real hardware device.
Can register itself, authenticate, send telemetry, listen for
remote commands via WebSocket, and report command results.

Usage:
  python3 device-sim.py                          # interactive prompts
  python3 device-sim.py --server http://10.0.0.5:8080
  python3 device-sim.py --auto --count 3          # auto-launch N devices

Environment:
  DEVICEOS_ADMIN_TOKEN    Admin API key (default: dos_dev_admin_token_0001)
  DEVICEOS_SERVER         Server URL (default: http://localhost:8080)
"""

import argparse
import json
import math
import os
import random
import signal
import sys
import textwrap
import threading
import time
import urllib.error
import urllib.request

HAS_WS = False
try:
    import websocket
    HAS_WS = True
except ImportError:
    pass


# ── Coloured terminal helpers ──────────────────────────────────────

class Col:
    GREY = "\033[90m"
    CYAN = "\033[96m"
    GREEN = "\033[92m"
    YELLOW = "\033[93m"
    RED = "\033[91m"
    MAGENTA = "\033[95m"
    BOLD = "\033[1m"
    RESET = "\033[0m"


def say(dev_id, tag, msg, colour=Col.GREY):
    ts = time.strftime("%H:%M:%S")
    name = dev_id[:12] if dev_id else "?"
    print(f"  {Col.GREY}{ts}{Col.RESET} [{colour}{tag}{Col.RESET}] {Col.BOLD}{name}{Col.RESET} {msg}")


# ── HTTP helpers ───────────────────────────────────────────────────

class DeviceClient:
    """HTTP + WebSocket client that mimics an IoT device."""

    def __init__(self, server: str, admin_token: str = ""):
        self.server = server.rstrip("/")
        self.admin_token = admin_token or os.environ.get("DEVICEOS_ADMIN_TOKEN", "")
        self.device_id = ""
        self.secret_key = ""
        self.token = ""
        self._stop = threading.Event()

    # ── low-level request ─────────────────────────────────────────
    def _req(self, method, path, body=None, token=None, raw=False, timeout=30):
        url = self.server + path
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = f"Bearer {token}"
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            resp = urllib.request.urlopen(req, timeout=timeout)
            body_bytes = resp.read()
            if raw:
                return resp.status, body_bytes
            return resp.status, json.loads(body_bytes) if body_bytes.strip() else {}
        except urllib.error.HTTPError as e:
            body_bytes = e.read()
            try:
                return e.code, json.loads(body_bytes)
            except Exception:
                return e.code, body_bytes.decode()
        except (urllib.error.URLError, TimeoutError, ConnectionError, OSError) as e:
            return 0, str(e)

    # ── health check ──────────────────────────────────────────────
    def health(self) -> dict:
        _, data = self._req("GET", "/healthz")
        return data

    # ── register this device ──────────────────────────────────────
    def register(self, name: str, dtype: str = "generic", metadata: dict = None) -> bool:
        status, data = self._req("POST", "/api/v1/devices", {
            "name": name,
            "type": dtype,
            "metadata": metadata or {},
        }, token=self.admin_token)
        if status == 201:
            dev = data.get("device", {})
            self.device_id = dev.get("id", "")
            self.secret_key = data.get("secret_key", "")
            return True
        return False

    # ── authenticate (get device JWT) ─────────────────────────────
    def auth(self) -> bool:
        if not self.device_id or not self.secret_key:
            return False
        status, data = self._req("POST", "/api/v1/auth/token", {
            "device_id": self.device_id,
            "secret_key": self.secret_key,
        })
        if status == 200:
            self.token = data.get("token", "")
            return bool(self.token)
        return False

    # ── send telemetry with retry ─────────────────────────────────
    def send_telemetry(self, metrics: dict) -> bool:
        if not self.token:
            return False
        for attempt in range(3):
            status, _ = self._req("POST", "/api/v1/telemetry", {
                "device_id": self.device_id,
                "metrics": metrics,
            }, token=self.token, timeout=15)
            if status == 201:
                return True
            if status == 0:
                # Connection error — server may be busy (rate limit).
                # Wait and retry once.
                time.sleep(3 * (attempt + 1))
                continue
            return False
        return False

    # ── report command result ─────────────────────────────────────
    def report_result(self, cmd_id: str, status_str: str = "completed",
                      result: dict = None) -> bool:
        if not self.token:
            return False
        s, _ = self._req("PUT", f"/api/v1/commands/{cmd_id}/result", {
            "status": status_str,
            "result": result or {},
        }, token=self.token)
        return s == 200

    # ── check OTA deployment status ────────────────────────────────
    def ota_device_status(self, deployment_id: str, dev_status: str = "completed") -> bool:
        if not self.token:
            return False
        s, _ = self._req("PUT", f"/api/v1/deployments/{deployment_id}/device-status", {
            "device_id": self.device_id,
            "status": dev_status,
        }, token=self.token)
        return s == 200


# ── Simulated sensor data generators ──────────────────────────────

SENSOR_TYPES = {
    "temp-sensor": {
        "metrics": lambda t: {
            "temperature": round(20 + 10 * math.sin(t * 0.1) + random.gauss(0, 1), 1),
            "humidity": round(50 + 15 * math.sin(t * 0.07) + random.gauss(0, 2), 1),
            "battery": max(0, 100 - (t % 86400) * 0.001),
        },
        "colour": Col.CYAN,
    },
    "gps-tracker": {
        "metrics": lambda t: {
            "latitude": round(23.81 + random.gauss(0, 0.01), 4),
            "longitude": round(90.41 + random.gauss(0, 0.01), 4),
            "speed": round(max(0, 30 + 20 * math.sin(t * 0.05) + random.gauss(0, 3)), 1),
            "battery": max(0, 100 - (t % 43200) * 0.002),
        },
        "colour": Col.GREEN,
    },
    "multi-sensor": {
        "metrics": lambda t: {
            "temperature": round(22 + 8 * math.sin(t * 0.08) + random.gauss(0, 1), 1),
            "humidity": round(55 + 10 * math.sin(t * 0.06) + random.gauss(0, 2), 1),
            "co2": round(400 + 100 * math.sin(t * 0.02) + random.gauss(0, 20)),
            "rssi": round(-50 + random.gauss(0, 5)),
            "battery": max(0, 100 - (t % 72000) * 0.0015),
        },
        "colour": Col.MAGENTA,
    },
    "switch": {
        "metrics": lambda t: {
            "state": 1 if int(t / 30) % 2 == 0 else 0,
            "power_w": round(5.0 + random.random() * 0.5, 2),
            "voltage": round(230 + random.gauss(0, 2), 1),
        },
        "colour": Col.YELLOW,
    },
}


def generate_metrics(dtype: str, tick: float) -> dict:
    gen = SENSOR_TYPES.get(dtype, SENSOR_TYPES["temp-sensor"])
    return gen["metrics"](tick)


# ── WebSocket command listener ────────────────────────────────────

def ws_command_listener(dev: DeviceClient, dtype: str, colour: str,
                        on_command=None, stop_event=None):
    if not HAS_WS:
        say(dev.device_id, "WS", "websocket-client not installed (pip install websocket-client)", Col.RED)
        return

    ws_url = dev.server.replace("http://", "ws://").replace("https://", "wss://")
    ws_url += f"/api/v1/ws/commands?device_id={dev.device_id}"

    headers = {"Authorization": f"Bearer {dev.token}"}
    while not stop_event.is_set():
        try:
            ws = websocket.WebSocketApp(
                ws_url,
                header=headers,
                on_message=lambda ws, msg: _handle_cmd(ws, msg, dev, dtype, colour, on_command),
                on_error=lambda ws, err: None,
                on_close=lambda ws, *a: None,
            )
            ws.run_forever(ping_interval=30, ping_timeout=10)
        except Exception:
            pass
        time.sleep(3)


def _handle_cmd(ws, msg, dev, dtype, colour, on_command):
    try:
        cmd = json.loads(msg)
        cmd_id = cmd.get("id", "")
        command = cmd.get("command", "")
        payload = cmd.get("payload", {})
        say(dev.device_id, "CMD", f"received: {command} {payload}", Col.YELLOW)

        # Simulate execution delay
        time.sleep(random.uniform(0.5, 2.0))

        # Generate plausible result based on command
        result = {"success": True, "output": f"{command} executed"}
        if command == "reboot":
            result["uptime_before"] = random.randint(100, 99999)
            result["output"] = "rebooting..."
        elif command == "update-config":
            result["config_applied"] = payload
        elif command == "ping":
            result["latency_ms"] = round(random.uniform(5, 50), 1)
        elif command == "collect-logs":
            result["log_count"] = random.randint(0, 50)
            result["size_kb"] = round(random.uniform(0.1, 10), 2)
        elif command == "set-frequency":
            result["frequency_set"] = payload.get("interval_s", 30)
        else:
            result["ack"] = True

        dev.report_result(cmd_id, "completed", result)
        say(dev.device_id, "CMD", f"completed: {command} -> {result.get('output', 'ok')}", Col.GREEN)

        if on_command:
            on_command(command, payload, result)

    except json.JSONDecodeError:
        pass


# ── Device simulation loop ────────────────────────────────────────

def run_device(server: str, admin_token: str, name: str, dtype: str,
               interval: int, colour: str, stop_event: threading.Event,
               auto_register: bool, metadata: dict = None,
               phase_offset: float = 0):
    dev = DeviceClient(server, admin_token)

    # 1. Register if needed
    if auto_register or not dev.device_id:
        if not dev.register(name, dtype, metadata):
            say("?", "FAIL", f"registration failed (is admin_token valid?)", Col.RED)
            return
        say(dev.device_id, "REG", f"registered as {Col.BOLD}{name}{Col.RESET} [{dtype}] key={dev.secret_key[:16]}...", Col.GREEN)

    # 2. Authenticate
    if not dev.auth():
        say(dev.device_id, "FAIL", "authentication failed", Col.RED)
        return
    say(dev.device_id, "AUTH", "JWT token obtained", Col.GREEN)

    # 3. Start command listener in background thread
    cmd_stop = threading.Event()
    cmd_thread = threading.Thread(
        target=ws_command_listener,
        args=(dev, dtype, colour, None, cmd_stop),
        daemon=True,
    )
    cmd_thread.start()

    # 4. Stagger first send so devices don't all hit rate limiter
    if phase_offset > 0:
        stop_event.wait(timeout=phase_offset)

    # 4. Telemetry loop
    tick = 0
    say(dev.device_id, "RUN", f"sending telemetry every {interval}s — Ctrl+C to stop", colour)
    try:
        while not stop_event.is_set():
            metrics = generate_metrics(dtype, tick)
            ok = dev.send_telemetry(metrics)
            if ok:
                preview = ", ".join(f"{k}={v}" for k, v in list(metrics.items())[:3])
                say(dev.device_id, "TEL", f"{preview}{' ...' if len(metrics) > 3 else ''}", colour)
            else:
                say(dev.device_id, "TEL", "send failed (server busy or down)", Col.RED)
            tick += interval
            stop_event.wait(timeout=interval)
    finally:
        cmd_stop.set()


# ── Standalone operation (one-shot telemetry) ─────────────────────

def send_once(server: str, admin_token: str, device_id: str, secret_key: str,
              metrics: dict):
    dev = DeviceClient(server, admin_token)
    dev.device_id = device_id
    dev.secret_key = secret_key
    if not dev.auth():
        print("auth failed")
        return False
    return dev.send_telemetry(metrics)


# ── Interactive mode ──────────────────────────────────────────────

def interactive_mode(server: str, admin_token: str):
    print(textwrap.dedent(f"""\
    {Col.BOLD}DeviceOS — Virtual Device Simulator{Col.RESET}
    {Col.GREY}Server: {server}{Col.RESET}
    """))

    # Step 1: Register
    print(f"{Col.CYAN}── Register a device ──{Col.RESET}")
    name = input(f"  Device name [{Col.GREY}sim-device-1{Col.RESET}]: ") or "sim-device-1"
    dtype = input(f"  Device type [{Col.GREY}temp-sensor{Col.RESET}] ({', '.join(SENSOR_TYPES.keys())}): ") or "temp-sensor"

    dev = DeviceClient(server, admin_token)
    if not dev.register(name, dtype):
        print(f"  {Col.RED}Registration failed{Col.RESET}")
        return
    print(f"  {Col.GREEN}Registered!{Col.RESET}")
    print(f"  ID:   {Col.BOLD}{dev.device_id}{Col.RESET}")
    print(f"  Key:  {Col.YELLOW}{dev.secret_key}{Col.RESET}")

    # Step 2: Authenticate
    print(f"\n{Col.CYAN}── Authenticate ──{Col.RESET}")
    if not dev.auth():
        print(f"  {Col.RED}Auth failed{Col.RESET}")
        return
    print(f"  {Col.GREEN}Authenticated{Col.RESET}")

    # Step 3: Start simulation
    print(f"\n{Col.CYAN}── Simulating ──{Col.RESET}")
    interval = int(input(f"  Interval (seconds) [{Col.GREY}5{Col.RESET}]: ") or "5")

    stop = threading.Event()

    def on_cmd(command, payload, result):
        print(f"  {Col.YELLOW}<< command: {command}{Col.RESET}")

    cmd_stop = threading.Event()
    cmd_thread = threading.Thread(
        target=ws_command_listener,
        args=(dev, dtype, Col.CYAN, on_cmd, cmd_stop),
        daemon=True,
    )
    cmd_thread.start()

    tick = 0
    try:
        while True:
            metrics = generate_metrics(dtype, tick)
            dev.send_telemetry(metrics)
            preview = ", ".join(f"{k}={v}" for k, v in metrics.items())
            print(f"  {Col.GREY}[{time.strftime('%H:%M:%S')}]{Col.RESET} TEL {preview}")
            tick += interval
            time.sleep(interval)
    except KeyboardInterrupt:
        print(f"\n  {Col.YELLOW}stopping{Col.RESET}")
        cmd_stop.set()


# ── Auto mode (headless N devices) ────────────────────────────────

def auto_mode(server: str, admin_token: str, count: int, interval: int):
    print(f"{Col.BOLD}DeviceOS — Auto-launching {count} virtual devices{Col.RESET}")
    print(f"{Col.GREY}Server: {server}  Interval: {interval}s{Col.RESET}\n")

    names = [
        "warehouse-a", "warehouse-b", "cold-storage-1", "cold-storage-2",
        "truck-001", "truck-002", "truck-003", "gateway-alpha",
        "sensor-grid-1", "sensor-grid-2", "hvac-unit-3", "power-metter-1",
        "weather-station-1", "water-tank-1", "air-quality-1",
    ]
    types = list(SENSOR_TYPES.keys())
    stop = threading.Event()

    threads = []
    for i in range(count):
        name = names[i % len(names)] + f"-{i+1}"
        dtype = types[i % len(types)]
        colour = SENSOR_TYPES[dtype]["colour"]
        phase = i * (interval / count)
        t = threading.Thread(
            target=run_device,
            args=(server, admin_token, name, dtype, interval, colour, stop, True),
            kwargs={"phase_offset": phase},
            daemon=True,
        )
        t.start()
        threads.append(t)
        time.sleep(0.5)  # stagger registrations + auth

    print(f"\n  {Col.GREEN}{count} devices launched.{Col.RESET}")
    print(f"  {Col.GREY}Press Ctrl+C to stop all{Col.RESET}\n")

    try:
        while any(t.is_alive() for t in threads):
            time.sleep(1)
    except KeyboardInterrupt:
        print(f"\n  {Col.YELLOW}Stopping all devices...{Col.RESET}")
        stop.set()
        for t in threads:
            t.join(timeout=2)
    print(f"  {Col.GREEN}Done.{Col.RESET}")


# ── Entry ─────────────────────────────────────────────────────────

def main():

    p = argparse.ArgumentParser(description="DeviceOS virtual device simulator")
    p.add_argument("--server", default=os.environ.get("DEVICEOS_SERVER", "http://localhost:8080"))
    p.add_argument("--admin-token", default=os.environ.get("DEVICEOS_ADMIN_TOKEN", ""))
    p.add_argument("--auto", action="store_true", help="headless auto-launch mode")
    p.add_argument("--count", type=int, default=3, help="number of devices in auto mode")
    p.add_argument("--interval", type=int, default=5, help="telemetry interval in seconds")
    p.add_argument("--device-id", help="existing device ID (one-shot)")
    p.add_argument("--secret-key", help="existing secret key (one-shot)")
    p.add_argument("--name", default="sim-device", help="device name")
    p.add_argument("--type", dest="dtype", default="temp-sensor", choices=list(SENSOR_TYPES.keys()))
    args = p.parse_args()

    # Resolve admin token: CLI > env > built-in dev default
    admin_token = args.admin_token or os.environ.get("DEVICEOS_ADMIN_TOKEN", "dos_dev_admin_token_0001")

    # One-shot telemetry mode (existing device)
    if args.device_id and args.secret_key:
        metrics = generate_metrics(args.dtype, time.time())
        ok = send_once(args.server, admin_token, args.device_id, args.secret_key, metrics)
        print(json.dumps({"sent": ok, "metrics": metrics}, indent=2))
        sys.exit(0 if ok else 1)

    # Auto mode (headless)
    if args.auto:
        auto_mode(args.server, admin_token, args.count, args.interval)
        return

    # Interactive mode (default)
    interactive_mode(args.server, admin_token)


if __name__ == "__main__":
    main()
