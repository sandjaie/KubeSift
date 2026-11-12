#!/usr/bin/env python3
"""Apply the explicit, owned Phase 1C fixtures to the two existing local clusters.
Never edits shop/billing, adopts collisions, creates clusters or changes current-context.
"""
import json
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
OWNER = 'explorer-v1'
NAMESPACE = 'kubesift-explorer'

def kubectl(context, *args, data=None):
    result = subprocess.run(['kubectl', '--context', context, *args], input=data, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(f'{context}: kubectl {args[0]} failed: {result.stderr.strip()}')
    return result.stdout

def apply():
    inventory = (ROOT / 'tests/fixtures/explorer/inventory.json').read_text()
    for suffix in ('a', 'b'):
        context = f'kind-dev-local-{suffix}'
        objects = json.loads(inventory.replace('MARKER', f'local-{suffix}'))['items']
        # Check every existing collision before making any changes in this cluster.
        for obj in objects:
            if obj['kind'] == 'Widget':
                crd = kubectl(context, 'get', 'crd', 'widgets.explorer.kubesift.dev', '--ignore-not-found', '-o', 'json')
                if not crd.strip():
                    continue
            metadata = obj['metadata']
            scope = ['-n', metadata['namespace']] if 'namespace' in metadata else []
            old = kubectl(context, *scope, 'get', obj['kind'], metadata['name'], '--ignore-not-found', '-o', 'json')
            if old.strip() and json.loads(old)['metadata'].get('labels', {}).get('kubesift.dev/fixture') != OWNER:
                raise RuntimeError(f'{context}: refusing unowned collision {obj["kind"]}/{metadata["name"]}')
        for obj in objects:
            metadata = obj['metadata']
            scope = ['-n', metadata['namespace']] if 'namespace' in metadata else []
            kubectl(context, *scope, 'apply', '-f', '-', data=json.dumps(obj))
            if obj['kind'] == 'CustomResourceDefinition':
                kubectl(context, 'wait', '--for=condition=Established', 'crd/widgets.explorer.kubesift.dev', '--timeout=60s')
        print(f'{context}: applied {len(objects)} owned objects in {NAMESPACE}; existing namespaces untouched')

if __name__ == '__main__':
    try:
        apply()
    except (RuntimeError, OSError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
