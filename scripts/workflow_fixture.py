"""Purpose: Supply deterministic OpenShift API contracts for desktop verification.
Used by: project-fixture.py when the workflow option is selected.
Responsibilities: Emulate scoped reads, dry-run, single writes and synthetic Build output.
Important: All state is process-local. No router, build or OLM controller runs here.
"""
import copy
from urllib.parse import urlsplit, parse_qs

ROUTE = "route.openshift.io/v1"
BUILD = "build.openshift.io/v1"
OLM = "operators.coreos.com/v1alpha1"
PACKAGE = "packages.operators.coreos.com/v1"
GROUP = "operators.coreos.com/v1"
RBAC = "rbac.authorization.k8s.io/v1"
V1 = "olm.operatorframework.io/v1"
RESOURCES = {
    ROUTE: {"routes": "Route"}, BUILD: {"buildconfigs": "BuildConfig", "builds": "Build",
    "builds/log": "BuildLog", "builds/clone": "BuildRequest", "buildconfigs/instantiate": "BuildRequest"},
    OLM: {"subscriptions": "Subscription", "installplans": "InstallPlan", "clusterserviceversions": "ClusterServiceVersion",
          "catalogsources": "CatalogSource"}, GROUP: {"operatorgroups": "OperatorGroup"},
    PACKAGE: {"packagemanifests": "PackageManifest"}, RBAC: {"clusterroles": "ClusterRole"},
    V1: {"clusterextensions": "ClusterExtension","clustercatalogs":"ClusterCatalog"},
}
NS = "team-a"
OBJECTS = {}

def obj(gv, resource, name, spec=None, status=None, ns=NS):
    data = {"apiVersion": gv, "kind": RESOURCES[gv][resource], "metadata": {
        "namespace": ns, "name": name, "uid": name+"-uid", "resourceVersion": "3"},
        "spec": spec or {}, "status": status or {}}
    OBJECTS[gv, resource, ns, name] = data
    return data

def seed():
    obj(ROUTE,"routes","store",{"host":"store.example.test","path":"/","to":{"kind":"Service","name":"store"},
        "alternateBackends":[{"kind":"Service","name":"canary","weight":0}],"tls":{"termination":"edge","key":"fixture-private-sentinel"}},
        {"ingress":[{"routerName":"local","conditions":[{"type":"Admitted","status":"True"}]}]})
    build={"source":{"type":"Git","git":{"uri":"https://fixture:fixture-private-sentinel@example.test/demo.git"}},
        "strategy":{"type":"Docker"}, "output":{"to":{"kind":"DockerImage","name":"example.test/demo:latest"}}}
    obj(BUILD,"buildconfigs","builder",build,{"lastVersion":2})
    obj(BUILD,"builds","builder-run",build,{"phase":"Running","config":{"kind":"BuildConfig","name":"builder","namespace":NS}})
    obj(GROUP,"operatorgroups","group",{},{"namespaces":[NS]})
    obj(OLM,"catalogsources","catalog")
    obj(PACKAGE,"packagemanifests","demo",{},{"catalogSource":"catalog","catalogSourceNamespace":NS,"packageName":"demo","channels":[{"name":"stable","currentCSV":"demo.v2","currentCSVDesc":{"installModes":[{"type":"OwnNamespace","supported":True}]}}]})
    obj(OLM,"clusterserviceversions","demo.v1",{},{"phase":"Succeeded"})
    obj(OLM,"subscriptions","demo",{"name":"demo","channel":"stable","source":"catalog","sourceNamespace":NS,"installPlanApproval":"Manual"},
        {"installedCSV":"demo.v1","currentCSV":"demo.v2","installPlanRef":{"kind":"InstallPlan","name":"plan","namespace":NS,"uid":"plan-uid"}})
    import json
    steps=[]
    for gv,kind,name in [(OLM,"ClusterServiceVersion","demo.v2"),(RBAC,"ClusterRole","demo-reader")]:
        group,version=gv.split("/")
        step={"group":group,"version":version,"kind":kind,"name":name,"manifest":json.dumps({"apiVersion":gv,"kind":kind,"metadata":{"name":name}})}
        steps.append({"resource":step,"resolving":"demo.v2","status":"Unknown"})
    obj(OLM,"installplans","plan",{"approval":"Manual","approved":False,"clusterServiceVersionNames":["demo.v2"]},{"phase":"RequiresApproval","plan":steps})

def groups():
    out={}
    for gv in RESOURCES:
        g,v=gv.split("/")
        out.setdefault(g,[]).append({"version":v,"groupVersion":gv})
    return [{"name":g,"versions":vs} for g,vs in out.items()]

def serve(h):
    path=urlsplit(h.path).path
    if path.startswith("/api/v1/namespaces/team-a/services/"):
        name=path.rsplit("/",1)[1]
        h.reply({"apiVersion":"v1","kind":"Service","metadata":{"name":name,"namespace":NS,"uid":"fixture-service"},"spec":{"type":"ClusterIP","ports":[{"name":"web","port":80,"protocol":"TCP"}]}})
        return True
    parts=path.strip("/").split("/")
    if len(parts)<3 or parts[0]!="apis": return False
    gv="/".join(parts[1:3])
    if gv not in RESOURCES:return False
    if len(parts)==3:
        h.reply({"kind":"APIResourceList","groupVersion":gv,"resources":[{"kind":k,"name":r,"namespaced":gv not in (RBAC,V1),"verbs":["get","list","create","patch","delete"]} for r,k in RESOURCES[gv].items()]})
        return True
    rest=parts[3:]
    ns=""
    if rest[0]=="namespaces" and len(rest)>=3:
        ns,rest=rest[1],rest[2:]
    resource=rest[0]
    if resource not in RESOURCES[gv]:h.status(404);return True
    if ns not in ("",NS):h.status(403);return True
    name=rest[1] if len(rest)>1 else ""
    key=(gv,resource,ns,name)
    current=OBJECTS.get(key)
    dry=parse_qs(urlsplit(h.path).query).get("dryRun")==["All"]
    if h.command=="GET":
        if not name:h.reply({"kind":RESOURCES[gv][resource]+"List","apiVersion":gv,"items":[o for (g,r,n,_),o in OBJECTS.items() if (g,r,n)==(gv,resource,ns)]})
        elif current is None:h.status(404)
        elif rest[-1]=="log":
            data=b"2026-11-22T00:00:00Z Synthetic Build output\n";h.send_response(200);h.send_header("Content-Type","text/plain");h.send_header("Content-Length",str(len(data)));h.end_headers();h.wfile.write(data)
        else:h.reply(current)
        return True
    body=h.body()
    if h.command=="POST":
        if name:
            if not current or rest[-1] not in ("instantiate","clone"):h.status(404);return True
            n="returned-"+str(len(OBJECTS));h.reply(obj(BUILD,"builds",n,current["spec"],{"phase":"New"}),201)
        else:
            name=body["metadata"]["name"];key=(gv,resource,ns,name)
            if key in OBJECTS:h.status(409);return True
            body["metadata"].update(uid=name+"-uid",resourceVersion="1")
            if not dry:OBJECTS[key]=body
            h.reply(body,201)
        return True
    if not current:h.status(404);return True
    if h.command=="DELETE":
        if body["preconditions"]!={"uid":current["metadata"]["uid"],"resourceVersion":current["metadata"]["resourceVersion"]}:h.status(409);return True
        if not dry:del OBJECTS[key]
        h.reply(current);return True
    if h.command=="PATCH":
        updated=copy.deepcopy(current)
        for op in body:
            keys=op["path"].strip("/").split("/");parent=updated
            for k in keys[:-1]:parent=parent[int(k)] if isinstance(parent,list) else parent[k]
            k=int(keys[-1]) if isinstance(parent,list) else keys[-1]
            if op["op"]=="test":
                if parent[k]!=op["value"]:h.status(409);return True
            elif op["op"]=="remove":del parent[k]
            else:parent[k]=op["value"]
        if not dry:updated["metadata"]["resourceVersion"]=str(int(updated["metadata"]["resourceVersion"])+1);OBJECTS[key]=updated
        h.reply(updated);return True
    h.status(405);return True
