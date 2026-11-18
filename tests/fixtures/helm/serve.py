#!/usr/bin/env python3
"""
Purpose: Serve synthetic private HTTPS and OCI charts on this development machine.
Used by: Packaged Phase 1E.2 desktop verification, never by the shipped application.
Responsibilities: Prepare two archives and a temporary TLS CA, then serve bounded fixture files.
Important: Loopback only; credentials are public synthetic fixture values and nothing is installed in a cluster.
"""
import argparse
import base64
import hashlib
import http.server
import io
import json
from pathlib import Path
import ssl
import subprocess
import tarfile

parser = argparse.ArgumentParser()
parser.add_argument("--output", type=Path, required=True)
parser.add_argument("--port", type=int, default=18443)
args = parser.parse_args()
args.output.mkdir(parents=True, exist_ok=True)
root = Path(__file__).parent / "management"
charts = {}
for version in ("1.0.0", "1.1.0"):
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w:gz") as archive:
        for path in sorted(root.rglob("*")):
            if not path.is_file():
                continue
            data = path.read_bytes()
            if path.name == "Chart.yaml":
                data = data.replace(b"version: 1.0.0", ("version: " + version).encode())
            entry = tarfile.TarInfo("kubesift-management/" + str(path.relative_to(root)))
            entry.size, entry.mode = len(data), 0o600
            archive.addfile(entry, io.BytesIO(data))
    charts[version] = buffer.getvalue()
    (args.output / ("management-" + version + ".tgz")).write_bytes(buffer.getvalue())


def digest(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


raw = charts["1.1.0"]
manifest = json.dumps({
    "schemaVersion": 2,
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "config": {"mediaType": "application/vnd.cncf.helm.config.v1+json", "digest": digest(b"{}"), "size": 2},
    "layers": [{"mediaType": "application/vnd.cncf.helm.chart.content.v1.tar+gzip", "digest": digest(raw), "size": len(raw)}],
}).encode()
index = ("apiVersion: v1\nentries:\n  kubesift-management:\n"
         "    - name: kubesift-management\n      version: 1.1.0\n      digest: " + digest(raw)[7:] +
         "\n      urls: [management-1.1.0.tgz]\n").encode()
cert, key = args.output / "ca.pem", args.output / "key.pem"
if not cert.exists():
    subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "2",
                    "-subj", "/CN=localhost", "-addext", "subjectAltName=DNS:localhost,IP:127.0.0.1",
                    "-keyout", str(key), "-out", str(cert)], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    key.chmod(0o600)


class Handler(http.server.BaseHTTPRequestHandler):
    def do_HEAD(self):
        self.do_GET()

    def do_GET(self):
        expected = "Basic " + base64.b64encode(b"fixture:synthetic-helm-password").decode()
        if self.headers.get("Authorization") != expected:
            self.send_response(401)
            self.send_header("WWW-Authenticate", 'Basic realm="fixture"')
            self.end_headers()
            return
        data, media = None, "application/octet-stream"
        if self.path == "/index.yaml":
            data = index
        elif self.path == "/management-1.1.0.tgz" or self.path.endswith("/blobs/" + digest(raw)):
            data = raw
        elif self.path in ("/v2/kubesift-management/manifests/1.1.0", "/v2/kubesift-management/manifests/" + digest(manifest)):
            data, media = manifest, "application/vnd.oci.image.manifest.v1+json"
        if data is None:
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", media)
        self.send_header("Content-Length", str(len(data)))
        if data is manifest:
            self.send_header("Docker-Content-Digest", digest(manifest))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(data)


server = http.server.ThreadingHTTPServer(("127.0.0.1", args.port), Handler)
tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
tls.load_cert_chain(cert, key)
server.socket = tls.wrap_socket(server.socket, server_side=True)
print(f"HTTPS https://127.0.0.1:{args.port}; OCI oci://127.0.0.1:{args.port}; chart kubesift-management:1.1.0", flush=True)
print("Synthetic credentials: fixture / synthetic-helm-password; CA:", cert.resolve(), flush=True)
print("OCI digest:", digest(manifest), flush=True)
try:
    server.serve_forever()
finally:
    server.server_close()
