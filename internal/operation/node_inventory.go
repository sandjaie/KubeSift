/*
Purpose: Classify the complete assigned Pod inventory for a reviewed Node drain.
Used by: Node review, per-eviction revalidation and final observation.
Responsibilities: Verify controlling owners and local storage before projecting safe rows.
Important: Every page and owner lookup must succeed before admitting any write.
*/
package operation

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

var nodeOwners = map[string]string{"apps/v1/ReplicaSet": "replicasets", "apps/v1/DaemonSet": "daemonsets", "apps/v1/StatefulSet": "statefulsets", "batch/v1/Job": "jobs", "v1/ReplicationController": "replicationcontrollers"}

func safeNodeName(s string) bool { return len(validation.IsDNS1123Subdomain(s)) == 0 }
func podPath(r NodePod) string   { return "/api/v1/namespaces/" + r.Namespace + "/pods/" + r.Name }
func (c operationClient) classify(ctx context.Context, p core.Pod, cache map[string]bool) (NodePod, error) {
	r := NodePod{Namespace: p.Namespace, Name: p.Name, UID: string(p.UID), State: "blocked"}
	if !safeNodeName(p.Name) || len(validation.IsDNS1123Label(p.Namespace)) != 0 || p.UID == "" || p.ResourceVersion == "" {
		return r, model.Fail("invalid_response", "Pod identity is incomplete")
	}
	if p.Annotations[core.MirrorPodAnnotationKey] != "" {
		r.State, r.Reason = "skipped", "Mirror Pod remains"
		return r, nil
	}
	var owners []meta.OwnerReference
	for _, o := range p.OwnerReferences {
		if o.Controller != nil && *o.Controller {
			owners = append(owners, o)
		}
	}
	daemon := len(owners) == 1 && owners[0].Kind == "DaemonSet"
	if !daemon && p.DeletionTimestamp != nil {
		r.State, r.Reason = "terminating", "Waiting for the reviewed Pod to disappear"
		return r, nil
	}
	if len(owners) != 1 {
		r.Reason = "Pod requires one verified controlling owner"
		return r, nil
	}
	o := owners[0]
	plural := nodeOwners[o.APIVersion+"/"+o.Kind]
	if plural == "" || !safeNodeName(o.Name) || o.UID == "" {
		r.Reason = "Unsupported or incomplete controlling owner"
		return r, nil
	}
	base := "/apis/" + o.APIVersion
	if o.APIVersion == "v1" {
		base = "/api/v1"
	}
	path := base + "/namespaces/" + p.Namespace + "/" + plural + "/" + o.Name
	r.owner = path + "/" + string(o.UID)
	valid, known := cache[r.owner]
	if !known {
		var obj meta.PartialObjectMetadata
		_, err := c.call(ctx, "GET", path, false, nil, &obj)
		valid = err == nil && obj.Kind == o.Kind && obj.APIVersion == o.APIVersion && obj.Name == o.Name && obj.Namespace == p.Namespace && obj.UID == o.UID && obj.DeletionTimestamp == nil
		cache[r.owner] = valid
	}
	if !valid {
		r.Reason = "Controlling owner could not be verified"
		return r, nil
	}
	if daemon {
		r.State, r.Reason = "skipped", "Verified DaemonSet Pod remains"
		return r, nil
	}
	for _, v := range p.Spec.Volumes {
		switch {
		case v.EmptyDir != nil:
			r.Reason = "Temporary Pod storage (emptyDir)"
		case v.HostPath != nil:
			r.Reason = "Node-local hostPath storage"
		case v.Ephemeral != nil:
			r.Reason = "Ephemeral claim tied to the Pod"
		case v.CSI != nil:
			r.Reason = "Inline CSI storage lifetime is unknown"
		}
		if r.Reason != "" {
			return r, nil
		}
	}
	r.State, r.Reason = "candidate", "Graceful eviction"
	return r, nil
}
func (c operationClient) inventory(ctx context.Context, node string) ([]NodePod, error) {
	rows := []NodePod{}
	uids := map[string]bool{}
	names := map[string]bool{}
	pages := map[string]bool{}
	owners := map[string]bool{}
	token, version := "", ""
	for {
		q := url.Values{"fieldSelector": {"spec.nodeName=" + node}, "limit": {strconv.Itoa(100)}}
		if token != "" {
			q.Set("continue", token)
		}
		var list core.PodList
		_, e := c.sendRequest(ctx, "GET", "/api/v1/pods", false, nil, "", &list, requestOptions{query: q})
		if e != nil {
			return nil, e
		}
		if list.Kind != "PodList" || list.APIVersion != "v1" || list.ResourceVersion == "" || (version != "" && version != list.ResourceVersion) || len(rows)+len(list.Items) > 500 || len(list.Items) > 100 {
			return nil, model.Fail("inventory", "Assigned Pod inventory is incomplete or exceeds 500 Pods")
		}
		version = list.ResourceVersion
		for _, p := range list.Items {
			r, e := c.classify(ctx, p, owners)
			if e != nil {
				return nil, e
			}
			key := r.Namespace + "/" + r.Name
			if p.Spec.NodeName != node || uids[r.UID] || names[key] {
				return nil, model.Fail("inventory", "Assigned Pod inventory changed or contains duplicate identities")
			}
			uids[r.UID], names[key] = true, true
			rows = append(rows, r)
		}
		token = list.Continue
		if token == "" {
			break
		}
		if pages[token] || len(pages) >= 500 || strings.TrimSpace(token) == "" {
			return nil, model.Fail("inventory", "Incomplete Pod inventory")
		}
		pages[token] = true
	}
	sort.Slice(rows, func(i, j int) bool { return podPath(rows[i]) < podPath(rows[j]) })
	return rows, nil
}
