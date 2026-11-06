#!/usr/bin/env python3
"""Purpose: Manage only explicitly authorized, run-owned Phase 1A fixtures.
Used by: scripts/local-clusters.sh up|down.
Responsibilities: Reuse primary A, create disposable B, isolate credentials and record immutable ownership.
Important: Do not run up without authorization for existing kind-dev-local/payments fixture mutations.
"""
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import uuid

ROOT = Path(__file__).resolve().parents[1]
STATE = ROOT / '.local/phase1a'
OWNER = STATE / 'ownership.json'
A, B, NS = 'kind-dev-local', 'kind-kubesift-b', 'payments'
NODE = 'kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5'
PLURALS = {'Pod':'pods','ConfigMap':'configmaps','ServiceAccount':'serviceaccounts','Role':'roles','RoleBinding':'rolebindings'}

def run(args, body=None, timeout=240):
    result = subprocess.run(args, input=body, capture_output=True, text=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError(f'{args[0]} {args[1]} failed: {result.stderr.strip()}')
    return result.stdout

def private(path, value):
    temp = path.with_name(path.name+'.tmp')
    descriptor = os.open(temp, os.O_WRONLY|os.O_CREAT|os.O_TRUNC, 0o600)
    with os.fdopen(descriptor,'w') as stream:
        stream.write(value)
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temp,path)
    path.chmod(0o600)

def save(owner):
    private(OWNER,json.dumps(owner,indent=2)+'\n')

def kc(context,*args,body=None):
    if context not in (A,B):
        raise RuntimeError('Unrecognized fixture context')
    config = STATE/('a.yaml' if context==A else 'b.yaml')
    return run(['kubectl','--kubeconfig',str(config),'--context',context,'--namespace',NS,*args],body)

def get(context,kind,name):
    text = kc(context,'get',f'{PLURALS[kind]}/{name}','--ignore-not-found=true','-o','json')
    return json.loads(text) if text.strip() else None

def container_id():
    result = subprocess.run(['docker','inspect','--format','{{.Id}}','kubesift-b-control-plane'],capture_output=True,text=True)
    return result.stdout.strip() if result.returncode==0 else None

def owned(owner,context,kind,name,obj):
    previous = next((r for r in owner['resources'] if r['context']==context and r['kind']==kind and r['name']==name),None)
    return previous and obj['metadata']['uid']==previous['uid'] and obj['metadata'].get('labels',{}).get('kubesift.dev/run')==owner['runId']

def validate_owner(owner):
    keys={'version','runId','contextA','contextB','phase','createdB','containerB','resources'}
    if set(owner)!=keys or owner['version']!=1 or owner['contextA']!=A or owner['contextB']!=B or owner['phase'] not in ['preparing','ready','stopped']:
        raise RuntimeError('Invalid fixture ownership schema')
    uuid.UUID(owner['runId'])
    allowed={(item['kind'],item['metadata']['name']) for item in json.loads((ROOT/'tests/fixtures/phase1a.yaml').read_text())['items']}
    for resource in owner['resources']:
        if set(resource)!={'context','kind','name','uid'} or resource['context'] not in [A,B] or (resource['kind'],resource['name']) not in allowed or not resource['uid']:
            raise RuntimeError('Invalid fixture resource ownership')
    if owner['createdB'] and (not isinstance(owner['containerB'],str) or len(owner['containerB'])!=64):
        raise RuntimeError('Invalid disposable container identity')

def up():
    os.umask(0o077)
    STATE.mkdir(parents=True,exist_ok=True)
    owner = json.loads(OWNER.read_text()) if OWNER.exists() else None
    if owner:
        validate_owner(owner)
    if owner and owner.get('phase')=='stopped':
        owner=None
    if owner is None:
        if not OWNER.exists() and any(STATE.iterdir()):
            raise RuntimeError('State has no ownership record; inspect instead of overwriting')
        owner={'version':1,'runId':str(uuid.uuid4()),'contextA':A,'contextB':B,'phase':'preparing','createdB':False,'containerB':None,'resources':[]}
        save(owner)
    clusters=run(['kind','get','clusters']).splitlines()
    if 'dev-local' not in clusters:
        raise RuntimeError('Approved primary dev-local is absent')
    if 'kubesift-b' in clusters and (not owner['createdB'] or owner['containerB']!=container_id()):
        raise RuntimeError('Pre-existing kubesift-b is not owned by this run')
    private(STATE/'a.yaml',run(['kind','get','kubeconfig','--name','dev-local']))
    template=json.loads((ROOT/'tests/fixtures/phase1a.yaml').read_text())
    for item in template['items']:
        obj=get(A,item['kind'],item['metadata']['name'])
        if obj and not owned(owner,A,item['kind'],item['metadata']['name'],obj):
            raise RuntimeError('Primary fixture collision; existing objects will not be adopted')
    if 'kubesift-b' not in clusters:
        print('Creating owned disposable B; primary A is reused',flush=True)
        run(['kind','create','cluster','--name','kubesift-b','--image',NODE,'--kubeconfig',str(STATE/'b.yaml'),'--wait','180s'],timeout=360)
        owner['createdB']=True
        owner['containerB']=container_id()
        save(owner)
    else:
        private(STATE/'b.yaml',run(['kind','get','kubeconfig','--name','kubesift-b']))
    (STATE/'b.yaml').chmod(0o600)
    if not kc(B,'get','namespace',NS,'--ignore-not-found=true','-o','name').strip():
        kc(B,'create','namespace',NS)
    for context,marker in [(A,'cluster-a'),(B,'cluster-b')]:
        for original in template['items']:
            item=copy.deepcopy(original)
            metadata=item['metadata']
            metadata['labels']={'kubesift.dev/validation':'phase1a','kubesift.dev/run':owner['runId']}
            if item['kind']=='ConfigMap':
                item['data']['marker']=marker
            if item['kind']=='Pod':
                metadata['annotations']['kubesift.dev/marker']=marker
                item['spec']['containers'][0]['env'][0]['value']=marker
            obj=get(context,item['kind'],metadata['name'])
            if obj:
                if not owned(owner,context,item['kind'],metadata['name'],obj):
                    raise RuntimeError('Fixture collision; no object adopted')
            else:
                created=json.loads(kc(context,'create','-f','-','-o','json',body=json.dumps(item)))
                owner['resources'].append({'context':context,'kind':item['kind'],'name':metadata['name'],'uid':created['metadata']['uid']})
                save(owner)
        kc(context,'wait','--for=condition=Ready','pod/ks-validation','--timeout=180s')
        print(f'{context}/{NS}: owned fixture ready',flush=True)
    a=json.loads(kc(A,'config','view','--raw','--flatten','-o','json'))
    b=json.loads(kc(B,'config','view','--raw','--flatten','-o','json'))
    config={'apiVersion':'v1','kind':'Config','clusters':a['clusters']+b['clusters'],'users':a['users']+b['users'],'contexts':a['contexts']+b['contexts'],'current-context':A}
    for context in config['contexts']:
        context['context']['namespace']=NS
    denied=kc(A,'create','token','ks-denied','--duration=1h').strip()
    token=kc(A,'create','token','ks-helper','--duration=1h').strip()
    private(STATE/'helper-token',token)
    run(['go','build','-o',str(STATE/'helper'),'./tests/fixtures/helper'])
    config['users'] += [{'name':'phase1a-denied','user':{'token':denied}},{'name':'phase1a-helper','user':{'exec':{'apiVersion':'client.authentication.k8s.io/v1','command':str(STATE/'helper'),'interactiveMode':'Never','env':[{'name':'KUBESIFT_HELPER_CREDENTIAL','value':str(STATE/'helper-token')},{'name':'KUBESIFT_HELPER_COUNTER','value':str(STATE/'helper-counter')}]}}}]
    cluster=a['contexts'][0]['context']['cluster']
    config['contexts'] += [{'name':name,'context':{'cluster':cluster,'user':name,'namespace':NS}} for name in ['phase1a-denied','phase1a-helper']]
    private(STATE/'kubeconfig',json.dumps(config))
    servers={c['name']:c['cluster']['server'] for c in config['clusters']}
    identities={c['name']:{'context':c['name'],'namespace':NS,'server':servers[c['context']['cluster']]} for c in config['contexts']}
    private(STATE/'kubeconfig.targets.json',json.dumps({'version':1,'runId':owner['runId'],'a':identities[A],'b':identities[B]},indent=2))
    owner['phase']='ready'
    save(owner)
    print('Isolated kubeconfig: .local/phase1a/kubeconfig',flush=True)

def down():
    if not OWNER.exists():
        raise RuntimeError('No ownership record; nothing will be deleted')
    owner=json.loads(OWNER.read_text())
    validate_owner(owner)
    if owner.get('phase')=='stopped':
        print('Owned fixtures already stopped')
        return
    actual=container_id()
    if owner['createdB'] and actual and actual!=owner['containerB']:
        raise RuntimeError('B identity changed; teardown refused')
    verified=[]
    for resource in owner['resources']:
        if resource['context']==B and actual is None:
            continue
        obj=get(resource['context'],resource['kind'],resource['name'])
        if obj:
            if not owned(owner,resource['context'],resource['kind'],resource['name'],obj):
                raise RuntimeError('Fixture UID changed; teardown refused')
            verified.append(resource)
    for resource in reversed(verified):
        prefix='/api/v1' if resource['kind'] in ['Pod','ConfigMap','ServiceAccount'] else '/apis/rbac.authorization.k8s.io/v1'
        uri=f"{prefix}/namespaces/{NS}/{PLURALS[resource['kind']]}/{resource['name']}"
        options={'apiVersion':'v1','kind':'DeleteOptions','preconditions':{'uid':resource['uid']},'propagationPolicy':'Background'}
        kc(resource['context'],'delete','--raw',uri,'-f','-',body=json.dumps(options))
    if owner['createdB'] and actual:
        run(['docker','rm','-f',owner['containerB']])
    owner['phase']='stopped'
    save(owner)
    for name in ['a.yaml','b.yaml','kubeconfig','helper-token','helper-counter','helper']:
        (STATE/name).unlink(missing_ok=True)
    print('Owned fixtures removed; primary cluster and namespace retained')

if __name__=='__main__':
    try:
        if len(sys.argv)!=2 or sys.argv[1] not in ['up','down']:
            raise RuntimeError('usage: local-clusters.sh up|down')
        up() if sys.argv[1]=='up' else down()
    except (RuntimeError,OSError,ValueError,KeyError,subprocess.TimeoutExpired) as error:
        print(f'Fixture operation stopped: {error}',file=sys.stderr)
        raise SystemExit(1)
