#!/usr/bin/env python3
"""Serve synthetic range evidence through a local TLS gateway and a pinned Prometheus engine.

Run with the private fixture directory and the extracted Prometheus 3.5.0 bin directory.
The caller owns cluster fixtures, the certificate/key and shutdown. Only loopback listeners are used.
"""
import hmac
import http.client
import http.server
import os
from pathlib import Path
import secrets
import socket
import ssl
import subprocess
import sys
import time
from urllib.parse import parse_qs, urlsplit

root, binaries = map(Path, sys.argv[1:])
os.umask(0o077)
token = secrets.token_urlsafe(32)
(root / "token").write_text(token)
start = int(time.time()) // 30 * 30 - 3600
(root / "series-start").write_text(str(start))
with (root / "samples.om").open("w") as stream:
    for metric in ("container_cpu_usage_seconds_total", "container_memory_working_set_bytes"):
        for index in range(240):
            for cluster, factor in (("a", 1), ("b", 2)):
                pod = f'cluster="{cluster}",namespace="kubesift-history-check",pod="historical-usage"'
                node = f'cluster="{cluster}",node="dev-local-{cluster}-control-plane",id="/"'
                for labels, scale in ((pod + ',container="app"', factor), (pod + ',container="sidecar"', 3 * factor), (node, 4 * factor)):
                    value = ((index % 8 + 1) * 6 if metric.endswith("total") else 32 * 1048576) * scale
                    print(f'{metric}{{{labels}}} {value} {start + 30 * index}', file=stream)
    print("# EOF", file=stream)
subprocess.run([binaries / "promtool", "tsdb", "create-blocks-from", "openmetrics", root / "samples.om", root / "data"], check=True)
(root / "config.yml").write_text("global:\n  scrape_interval: 1m\nscrape_configs: []\n")
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    engine_port = sock.getsockname()[1]

class Gateway(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        path = urlsplit(self.path)
        if path.path != "/api/v1/query_range":
            return self.send_error(404)
        if not hmac.compare_digest(self.headers.get("Authorization", ""), "Bearer " + token):
            return self.send_error(403)
        namespace = parse_qs(path.query).get("namespace")
        if namespace is not None and namespace != ["kubesift-history-check"]:
            return self.send_error(403)
        upstream = http.client.HTTPConnection("127.0.0.1", engine_port, timeout=10)
        try:
            upstream.request("GET", self.path)
            answer = upstream.getresponse()
            body = answer.read()
            self.send_response(answer.status)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(body)
        finally:
            upstream.close()

with http.server.ThreadingHTTPServer(("127.0.0.1", 0), Gateway) as server:
    tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    tls.load_cert_chain(root / "server.pem", root / "server.key")
    server.socket = tls.wrap_socket(server.socket, server_side=True)
    engine = subprocess.Popen([binaries / "prometheus", "--config.file=" + str(root / "config.yml"),
                               "--storage.tsdb.path=" + str(root / "data"), f"--web.listen-address=127.0.0.1:{engine_port}"])
    try:
        (root / "origin").write_text(f"https://127.0.0.1:{server.server_port}")
        server.serve_forever()
    finally:
        engine.terminate()
        engine.wait()
