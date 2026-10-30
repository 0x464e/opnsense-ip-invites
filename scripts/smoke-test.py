#!/usr/bin/env python3
"""Exercise a built production executable against an isolated mock firewall."""
import base64
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError
from urllib.request import Request, urlopen


def main():
    binary = Path(sys.argv[1] if len(sys.argv) > 1 else "dist/opnsense-ip-invites").resolve()
    if not binary.is_file():
        raise SystemExit("Build the executable with make build first.")
    table = set()

    class Firewall(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def respond(self, data, status=200):
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(data).encode())

        def do_GET(self):
            expected = "Basic " + base64.b64encode(b"test-key:test-secret").decode()
            if self.headers.get("Authorization") != expected:
                return self.respond({}, 401)
            if self.path == "/api/firewall/alias_util/aliases":
                return self.respond(["SMOKE_TEST"])
            if self.path.startswith("/api/firewall/alias/search_item?"):
                rows = [{"name": "SMOKE_TEST", "type": "external", "enabled": "1"}]
            elif self.path.startswith("/api/firewall/alias_util/list/SMOKE_TEST?"):
                rows = [{"ip": ip} for ip in sorted(table)]
            else:
                return self.respond({}, 404)
            self.respond({"rows": rows, "total": len(rows)})

        def do_POST(self):
            value = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            if self.path == "/api/firewall/alias_util/add/SMOKE_TEST":
                table.add(value["address"])
            elif self.path == "/api/firewall/alias_util/delete/SMOKE_TEST":
                table.discard(value["address"])
            else:
                return self.respond({}, 404)
            self.respond({"status": "done"})

    firewall = ThreadingHTTPServer(("127.0.0.1", 0), Firewall)
    thread = threading.Thread(target=firewall.serve_forever, daemon=True)
    thread.start()
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    origin = f"http://127.0.0.1:{port}"
    admin_secret = "smoke-test-proxy-secret-not-for-production"
    process = None
    try:
        with tempfile.TemporaryDirectory(prefix="ip-invites-smoke-") as temp:
            config = Path(temp) / "config.yaml"
            config.write_text(f"""server:
  listen: "127.0.0.1:{port}"
  public_url: "{origin}"
  trusted_proxies: ["127.0.0.1/32"]
  client_ip_header: "X-Real-IP"
database: "./invites.db"
defaults:
  link_validity: "72h"
  access_duration: "45m"
  max_ips: 3
opnsense:
  url: "http://127.0.0.1:{firewall.server_port}"
  timeout: "1s"
sync_interval: "1s"

""")
            env = os.environ.copy()
            for name in ["OPNSENSE_API_KEY", "OPNSENSE_API_SECRET", "INVITES_ADMIN_PROXY_SECRET"]:
                env.pop(name + "_FILE", None)
            env.update(OPNSENSE_API_KEY="test-key", OPNSENSE_API_SECRET="test-secret", INVITES_ADMIN_PROXY_SECRET=admin_secret)
            process = subprocess.Popen([str(binary), "-config", str(config)], env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)

            def request(path, body=None, admin=False):
                headers = {"X-Real-IP": "203.0.113.42", "Origin": origin}
                if admin:
                    headers["X-Invites-Admin-Secret"] = admin_secret
                if body is not None:
                    headers["Content-Type"] = "application/json"
                req = Request(origin + path, data=None if body is None else json.dumps(body).encode(), headers=headers)
                try:
                    with urlopen(req, timeout=5) as response:
                        return response.status, response.read().decode()
                except HTTPError as error:
                    return error.code, error.read().decode()

            for attempt in range(100):
                if process.poll() is not None:
                    raise AssertionError(process.stdout.read().decode())
                try:
                    if request("/readyz")[0] == 200:
                        break
                except OSError:
                    pass
                time.sleep(0.1)
            else:
                raise AssertionError("Production server did not become ready")
            assert request("/admin")[0] == 403
            assert request("/admin%2Fsettings")[0] == 403
            status, html = request("/admin", admin=True)
            assert status == 200 and "OPNsense IP Invites" in html and "/assets/" in html
            status, data = request("/api/admin/lists", {"name": "Smoke test", "alias": "SMOKE_TEST"}, admin=True)
            assert status == 201
            list_id = json.loads(data)["list"]["id"]
            status, data = request("/api/admin/links", {"list_id": list_id, "label": "Smoke invitation"}, admin=True)
            created = json.loads(data)
            assert status == 201 and created["link"]["access_seconds"] == 2700 and created["link"]["max_ips"] == 3
            token = created["url"].rsplit("/", 1)[1]
            assert request(f"/api/public/invites/{token}")[0] == 200 and not table
            status, data = request(f"/api/public/invites/{token}/authorize", {})
            assert status == 200 and json.loads(data)["ip"] == "203.0.113.42" and table == {"203.0.113.42"}
            assert request(f'/api/admin/links/{created["link"]["id"]}/revoke', {"revoke_grants": True}, admin=True)[0] == 200
            assert not table
            process.terminate()
            assert process.wait(timeout=10) == 0
            process = None
            print("Production smoke test passed: embedded UI, admin boundary, YAML defaults, authorization, revocation, and graceful shutdown.")
    finally:
        if process is not None and process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        firewall.shutdown()
        firewall.server_close()
        thread.join(timeout=5)


if __name__ == "__main__":
    main()
