#!/bin/sh
# Purpose: Opt into an owned Metrics Server development fixture on the approved kind cluster.
# Used by: Phase 1E.3 validation, never the shipped application.
# Responsibilities: Pin upstream bytes, refuse collisions and clean up only recorded UIDs.
# Important: Kubelet TLS relaxation below is solely for disposable kind development.
set -eu
cd "$(dirname "$0")/.."
exec python3 - "$@" <<'PY'
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import urllib.request

ROOT = Path('.superpowers/sdd/2026-11-19-phase-1e3-current-metrics')
URL = 'https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.8.1/components.yaml'
SHA = '4a672c4891902573a3ff753cece5de1bf1f55dd053403dfec39df9d1636b7ff1'
IMAGE = 'registry.k8s.io/metrics-server/metrics-server:v0.8.1@sha256:b2d2efaf5ac3b366ed0f839d2412a2c4279d4fc2a2a733f12c52133faed36c41'

def context_guard(context):
    if context != 'kind-dev-local-a':
        raise ValueError('Provider fixture setup/cleanup is restricted to kind-dev-local-a')

def path_for(obj):
    version, kind, meta = obj['apiVersion'], obj['kind'], obj['metadata']
    plural = {'ServiceAccount':'serviceaccounts', 'ClusterRole':'clusterroles',
              'ClusterRoleBinding':'clusterrolebindings', 'RoleBinding':'rolebindings',
              'Service':'services', 'Deployment':'deployments', 'APIService':'apiservices'}[kind]
    prefix = '/api/v1' if version == 'v1' else '/apis/' + version
    namespace = '/namespaces/' + meta['namespace'] if meta.get('namespace') else ''
    return prefix + namespace + '/' + plural + '/' + meta['name']

def no_collisions(objects, read):
    for obj in objects:
        if read(path_for(obj)):
            raise ValueError('Refusing to overwrite existing ' + path_for(obj))

def delete_owned(records, read, delete):
    for record in reversed(records):
        live = read(record['path'])
        if live and live['metadata']['uid'] != record['uid']:
            raise ValueError('Fixture identity changed; refusing cleanup: ' + record['path'])
        if live:
            delete(record['path'], {'apiVersion':'v1','kind':'DeleteOptions',
                   'preconditions':{'uid':record['uid']}})

if sys.argv[1:] == ['--self-test']:
    try:
        context_guard('production')
        assert False
    except ValueError:
        pass
    obj = {'apiVersion':'v1','kind':'Service','metadata':{'name':'metrics-server','namespace':'kube-system'}}
    try:
        no_collisions([obj], lambda path: obj)
        assert False
    except ValueError:
        pass
    deleted = []
    records = [{'path':path_for(obj),'uid':'owned'}]
    delete_owned(records, lambda path: {'metadata':{'uid':'owned'}}, lambda path, body: deleted.append((path, body)))
    assert deleted[0][1]['preconditions']['uid'] == 'owned'
    try:
        delete_owned(records, lambda path: {'metadata':{'uid':'replacement'}}, lambda *args: (_ for _ in ()).throw(AssertionError()))
        assert False
    except ValueError:
        pass
    print('Fixture guards and UID-only cleanup passed')
    sys.exit(0)

if len(sys.argv) != 3 or sys.argv[1] not in ('setup','cleanup'):
    sys.exit('Usage: scripts/metrics-fixtures.sh setup|cleanup kind-dev-local-a; or --self-test')
action, context = sys.argv[1:]
context_guard(context)
ROOT.mkdir(parents=True, exist_ok=True)
state = ROOT / 'provider-owned.json'

def kubectl(*args, data=None, check=True):
    result = subprocess.run(['kubectl','--context',context,*args], input=data, text=True, capture_output=True)
    if check and result.returncode:
        raise RuntimeError(result.stderr.strip())
    return result

def read(path):
    result = kubectl('get','--raw',path,check=False)
    if result.returncode:
        if '(NotFound)' in result.stderr:
            return None
        raise RuntimeError(result.stderr.strip())
    return json.loads(result.stdout)

if action == 'cleanup':
    if not state.exists():
        sys.exit('No owned provider inventory; nothing deleted')
    inventory = json.loads(state.read_text())
    assert inventory['context'] == context
    delete_owned(inventory['resources'], read, lambda path, body: kubectl('delete','--raw',path,'-f','-',data=json.dumps(body)))
    state.unlink()
    print('Removed recorded provider UIDs only')
else:
    if state.exists():
        sys.exit('Owned provider inventory already exists; inspect or clean it first')
    if read('/apis/metrics.k8s.io'):
        sys.exit('Metrics API already served; reuse the existing provider')
    raw = urllib.request.urlopen(URL, timeout=30).read()
    assert hashlib.sha256(raw).hexdigest() == SHA, 'Pinned manifest checksum mismatch'
    (ROOT/'components.yaml').write_bytes(raw)
    stream = kubectl('create','--dry-run=client','--validate=false','-f','-','-o','json',data=raw.decode()).stdout.strip()
    objects = []
    decoder = json.JSONDecoder()
    while stream:
        obj, end = decoder.raw_decode(stream)
        objects.extend(obj['items'] if obj.get('kind') == 'List' else [obj])
        stream = stream[end:].strip()
    no_collisions(objects, read)
    inventory = {'context':context,'manifestSHA256':SHA,'image':IMAGE,'resources':[]}
    state.write_text(json.dumps(inventory, indent=2))
    for obj in objects:
        if obj['kind'] == 'Deployment':
            container = obj['spec']['template']['spec']['containers'][0]
            container['image'] = IMAGE
            container['args'].append('--kubelet-insecure-tls')
        created = json.loads(kubectl('create','-f','-','-o','json',data=json.dumps(obj)).stdout)
        inventory['resources'].append({'path':path_for(created),'uid':created['metadata']['uid']})
        state.write_text(json.dumps(inventory, indent=2))
    print('Created pinned opt-in provider fixture; owned inventory: ' + str(state))
PY
