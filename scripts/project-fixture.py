#!/usr/bin/env python3
"""Purpose: Serve deterministic Project API contracts for packaged desktop checks.
Used by: Phase 1F.1 validation with a temporary kubeconfig directory.
Responsibilities: Emulate restricted reads, a self-service request and conditional deletion.
Important: Synthetic loopback state only. This server cannot contact a cluster.
"""
import argparse
import base64
import ssl
import subprocess
import json
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from urllib.parse import urlsplit

VERSION = "project.openshift.io/v1"
ROOT = "/apis/" + VERSION
NAMES = {"team-a": "fixture-a", "team-b": "fixture-b"}

def project(name):
    return {"apiVersion": VERSION, "kind": "Project",
            "metadata": {"name": name, "resourceVersion": "7", "uid": NAMES[name],
                         "annotations": {"openshift.io/display-name": name.title(),
                                         "openshift.io/description": "Synthetic desktop fixture"}},
            "status": {"phase": "Active"}}

class Handler(BaseHTTPRequestHandler):
    def reply(self, data, code=200):
        raw = json.dumps(data).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def status(self, code=200):
        return self.reply({"apiVersion": "v1", "kind": "Status", "code": code,
                           "reason": {403: "Forbidden", 404: "NotFound", 409: "Conflict"}.get(code, ""),
                           "status": "Success" if code == 200 else "Failure"}, code)

    def do_GET(self):
        path = urlsplit(self.path).path
        if path == "/api":
            return self.reply({"kind": "APIVersions", "versions": ["v1"]})
        if path == "/apis":
            return self.reply({"kind": "APIGroupList", "groups": [
                {"name": "project.openshift.io", "versions": [{"version": "v1", "groupVersion": VERSION}]}]})
        if path == ROOT:
            return self.reply({"kind": "APIResourceList", "groupVersion": VERSION, "resources": [
                {"kind": "Project", "name": "projects", "namespaced": False, "verbs": ["list", "get", "delete"]},
                {"kind": "ProjectRequest", "name": "projectrequests", "namespaced": False, "verbs": ["list", "create"]}]})
        if path == ROOT + "/projects":
            return self.reply({"apiVersion": VERSION, "kind": "ProjectList", "items": [project("team-a")] if "team-a" in NAMES else []})
        if path == ROOT + "/projectrequests":
            return self.status()
        if path.startswith(ROOT + "/projects/"):
            name = path.rsplit("/", 1)[1]
            return self.reply(project(name)) if name in NAMES else self.status(404)
        if path == "/api/v1/namespaces/team-a/resourcequotas":
            return self.reply({"apiVersion": "v1", "kind": "ResourceQuotaList", "items": [{
                "metadata": {"name": "team-limit", "namespace": "team-a", "uid": "quota-1"},
                "status": {"hard": {"pods": "12"}, "used": {"pods": "2"}}}]})
        return self.status(403)

    def body(self):
        if self.headers.get("Transfer-Encoding", "").lower() == "chunked":
            raw = bytearray()
            while True:
                size = int(self.rfile.readline(100).strip(), 16)
                if not size:
                    self.rfile.readline(2)
                    break
                if len(raw) + size > 32768:
                    raise ValueError("Fixture request exceeds 32 KiB")
                raw.extend(self.rfile.read(size))
                self.rfile.readline(2)
        else:
            size = int(self.headers["Content-Length"])
            if not 0 < size <= 32768:
                raise ValueError("Invalid fixture request size")
            raw = self.rfile.read(size)
        return json.loads(raw)

    def do_POST(self):
        path = urlsplit(self.path).path
        body = self.body()
        if path.endswith("/selfsubjectaccessreviews"):
            attrs = body["spec"]["resourceAttributes"]
            if attrs.get("namespace") == "team-b":
                return self.status(403)
            allowed = attrs["resource"] != "pods"
            return self.reply({"kind": "SelfSubjectAccessReview", "apiVersion": "authorization.k8s.io/v1",
                               "status": {"allowed": allowed, "denied": not allowed}})
        if path == ROOT + "/projectrequests":
            name = body["metadata"]["name"]
            if name in NAMES:
                return self.status(409)
            NAMES[name] = "requested-" + name
            return self.reply(project(name), 201)
        return self.status(403)

    def do_DELETE(self):
        path = urlsplit(self.path).path
        name = path.rsplit("/", 1)[-1]
        if path != ROOT + "/projects/" + name or name not in NAMES:
            return self.status(404)
        body = self.body()
        if body.get("preconditions") != {"uid": NAMES[name], "resourceVersion": "7"}:
            return self.status(409)
        if body.get("dryRun") != ["All"]:
            del NAMES[name]
        return self.status()

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("directory", type=Path)
    directory = parser.parse_args().directory
    server = HTTPServer(("127.0.0.1", 0), Handler)
    cert, key = directory / "tls.crt", directory / "tls.key"
    subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
                    "-subj", "/CN=localhost", "-addext", "subjectAltName=IP:127.0.0.1",
                    "-out", str(cert), "-keyout", str(key)], check=True, stderr=subprocess.DEVNULL)
    tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    tls.load_cert_chain(cert, key)
    server.socket = tls.wrap_socket(server.socket, server_side=True)
    cluster = "openshift-project-fixture"
    config = {"kind": "Config", "apiVersion": "v1", "clusters": [{"name": cluster, "cluster": {
        "certificate-authority-data": base64.b64encode(cert.read_bytes()).decode(), "server": f"https://127.0.0.1:{server.server_port}"}}], "users": [{"name": "reader", "user": {"token": "fixture-only"}}],
        "contexts": [{"name": cluster, "context": {"cluster": cluster, "user": "reader", "namespace": "unverified"}}]}
    with (directory / "projects.kubeconfig").open("x") as out:
        json.dump(config, out)
    server.serve_forever()
