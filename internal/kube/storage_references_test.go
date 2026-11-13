/*
Purpose: Exercise storage relationship allowlists and binding identity.
Used by: Storage resolver regression tests.
Responsibilities: Assert the source is re-read before any permitted target GET.
Important: All identities and responses are synthetic.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestStorageReferenceRoutes(t *testing.T) {
	const claim = "/api/v1/namespaces/test/persistentvolumeclaims/claim"
	const volume = "/api/v1/persistentvolumes/volume"
	const class = "/apis/storage.k8s.io/v1/storageclasses/class"
	const driver = "/apis/storage.k8s.io/v1/csidrivers/driver"
	const claimSpec = `{"storageClassName":"class","volumeName":"volume"}`
	const volumeSpec = `{"storageClassName":"class","claimRef":{"namespace":"test","name":"claim","uid":"target"},"csi":{"driver":"driver"}}`
	for _, tc := range []struct {
		source, target, sourcePath, targetPath, spec, targetSpec, identity, binding string
	}{
		{"Pod", "PersistentVolumeClaim", "/api/v1/namespaces/test/pods/pod", claim, `{"volumes":[{"name":"v","persistentVolumeClaim":{"claimName":"claim"}}]}`, claimSpec, "name-resolved", "not-applicable"},
		{"PersistentVolumeClaim", "PersistentVolume", claim, volume, claimSpec, strings.ReplaceAll(volumeSpec, "target", "source"), "name-resolved", "references-agree"},
		{"PersistentVolumeClaim", "StorageClass", claim, class, claimSpec, "{}", "name-resolved", "not-applicable"},
		{"PersistentVolume", "PersistentVolumeClaim", volume, claim, volumeSpec, claimSpec, "uid-verified", "references-agree"},
		{"PersistentVolume", "StorageClass", volume, class, volumeSpec, "{}", "name-resolved", "not-applicable"},
		{"PersistentVolume", "CSIDriver", volume, driver, volumeSpec, "{}", "name-resolved", "not-applicable"},
		{"VolumeAttachment", "PersistentVolume", "/apis/storage.k8s.io/v1/volumeattachments/attachment", volume, `{"source":{"persistentVolumeName":"volume"}}`, volumeSpec, "name-resolved", "not-applicable"},
		{"VolumeAttachment", "CSIDriver", "/apis/storage.k8s.io/v1/volumeattachments/attachment", driver, `{"attacher":"driver"}`, "{}", "name-resolved", "not-applicable"},
		{"CSINode", "CSIDriver", "/apis/storage.k8s.io/v1/csinodes/node", driver, `{"drivers":[{"name":"driver"}]}`, "{}", "name-resolved", "not-applicable"},
		{"PersistentVolumeClaim", "PersistentVolume", claim, volume, claimSpec, "{}", "name-resolved", "unconfirmed"},
		{"PersistentVolumeClaim", "PersistentVolume", claim, volume, claimSpec, `{"claimRef":{"namespace":"test","name":"claim"}}`, "name-resolved", "unconfirmed"},
		{"PersistentVolumeClaim", "PersistentVolume", claim, volume, claimSpec, volumeSpec, "", ""},
		{"PersistentVolumeClaim", "PersistentVolume", claim, volume, claimSpec, `{"claimRef":{"namespace":"elsewhere","name":"claim"}}`, "", ""},
		{"PersistentVolume", "PersistentVolumeClaim", volume, claim, volumeSpec, `{"volumeName":"elsewhere"}`, "", ""},
		{"PersistentVolume", "PersistentVolumeClaim", volume, claim, volumeSpec, "{}", "uid-verified", "unconfirmed"},
	} {
		t.Run(tc.source+"-"+tc.target+"-"+tc.binding, func(t *testing.T) {
			ref := func(kind, path, uid string) ResourceRef {
				parts := strings.Split(path, "/")
				version, ns := "v1", ""
				if parts[1] == "apis" {
					version = parts[2] + "/" + parts[3]
				}
				if parts[3] == "namespaces" {
					ns = parts[4]
				}
				return ResourceRef{Kind: kind, APIVersion: version, Namespace: ns, Name: parts[len(parts)-1], UID: uid}
			}
			source, target := ref(tc.source, tc.sourcePath, "source"), ref(tc.target, tc.targetPath, "")
			targetCalls := 0
			s, connection := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case tc.sourcePath:
					fmt.Fprintf(w, `{"kind":%q,"apiVersion":%q,"metadata":{"namespace":%q,"name":%q,"uid":"source","resourceVersion":"7"},"spec":%s}`, source.Kind, source.APIVersion, source.Namespace, source.Name, tc.spec)
				case tc.targetPath:
					targetCalls++
					fmt.Fprintf(w, `{"kind":%q,"apiVersion":%q,"metadata":{"namespace":%q,"name":%q,"uid":"target","resourceVersion":"8"},"spec":%s}`, target.Kind, target.APIVersion, target.Namespace, target.Name, tc.targetSpec)
				default:
					t.Errorf("unexpected GET %s", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			source.Connection, target.Connection = connection, connection
			ctx := context.Background()
			req := StorageReferenceRequest{Source: source, SourceVersion: "7", Target: target}
			for _, forge := range []func(*StorageReferenceRequest){
				func(r *StorageReferenceRequest) { r.SourceVersion = "6" },
				func(r *StorageReferenceRequest) { r.Source.UID = "replaced" },
				func(r *StorageReferenceRequest) { r.Target.Name = "elsewhere" },
				func(r *StorageReferenceRequest) { r.Target.APIVersion = "v2" },
				func(r *StorageReferenceRequest) { r.Target.Namespace = "elsewhere" },
				func(r *StorageReferenceRequest) { r.Target.Connection.Generation++ },
			} {
				bad := req
				forge(&bad)
				if _, err := s.ResolveStorageReference(ctx, bad); err == nil || targetCalls != 0 {
					t.Fatal("forged or stale request reached target", bad, err)
				}
			}
			result, err := s.ResolveStorageReference(ctx, req)
			if tc.binding == "" {
				if err == nil || targetCalls != 1 {
					t.Fatal("binding conflict accepted", result, err, targetCalls)
				}
				return
			}
			if err != nil || targetCalls != 1 || result.Resource.UID != "target" ||
				result.SourceVersion != "7" || result.TargetVersion != "8" ||
				result.IdentityEvidence != tc.identity || result.BindingEvidence != tc.binding {
				t.Fatal(result, err, targetCalls)
			}
		})
	}
}
