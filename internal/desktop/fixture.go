/*
Purpose: Restrict the validation write UI to recorded disposable local fixtures.
Used by: The desktop service when opening a connection and preparing marker reviews.
Responsibilities: Match the isolated source, TLS loopback endpoints and ownership journal before issuing a fixed marker target.
Important: A context label or user-entered namespace alone never authorizes this validation write.
*/
package desktop

import (
	"encoding/json"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"

	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/model"
)

type FixtureTarget struct {
	Target   model.Target `json:"target"`
	Cluster  string       `json:"cluster"`
	Endpoint string       `json:"endpoint"`
	UID      string       `json:"uid"`
}
type fixtureIdentity struct {
	Context   string `json:"context"`
	Namespace string `json:"namespace"`
	Server    string `json:"server"`
}

func localFixture(selected config.Resolved) (FixtureTarget, error) {
	denied := model.Fail("invalid", "Validation writes require the recorded isolated local fixture configuration")
	path := os.Getenv("KUBESIFT_TEST_KUBECONFIG")
	if !filepath.IsAbs(path) {
		return FixtureTarget{}, denied
	}
	var mapping struct {
		Version int             `json:"version"`
		RunID   string          `json:"runId"`
		A       fixtureIdentity `json:"a"`
		B       fixtureIdentity `json:"b"`
	}
	if readFixtureJSON(path+".targets.json", &mapping) != nil || mapping.Version != 1 || mapping.RunID == "" || mapping.A.Context != "kind-dev-local" || mapping.B.Context != "kind-kubesift-b" || mapping.A.Namespace != "payments" || mapping.B.Namespace != "payments" || mapping.A.Server == mapping.B.Server {
		return FixtureTarget{}, denied
	}
	for _, identity := range []fixtureIdentity{mapping.A, mapping.B} {
		endpoint, err := url.Parse(identity.Server)
		if err != nil {
			return FixtureTarget{}, denied
		}
		ip := net.ParseIP(endpoint.Hostname())
		if ip == nil || !ip.IsLoopback() || endpoint.Scheme != "https" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") {
			return FixtureTarget{}, denied
		}
	}
	snapshot, err := config.Load([]string{path})
	if err != nil {
		return FixtureTarget{}, denied
	}
	allowed, err := snapshot.Resolve(selected.Name())
	if err != nil || allowed.Source() != selected.Source() || !reflect.DeepEqual(allowed.RESTConfig(), selected.RESTConfig()) {
		return FixtureTarget{}, denied
	}
	identity := mapping.A
	if selected.Name() != identity.Context {
		identity = mapping.B
	}
	if selected.Name() != identity.Context || selected.RESTConfig().Host != identity.Server {
		return FixtureTarget{}, denied
	}
	var owner struct {
		Version    int    `json:"version"`
		RunID      string `json:"runId"`
		ContextA   string `json:"contextA"`
		ContextB   string `json:"contextB"`
		Phase      string `json:"phase"`
		CreatedB   bool   `json:"createdB"`
		ContainerB string `json:"containerB"`
		Resources  []struct {
			Context string `json:"context"`
			Kind    string `json:"kind"`
			Name    string `json:"name"`
			UID     string `json:"uid"`
		} `json:"resources"`
	}
	if readFixtureJSON(filepath.Join(filepath.Dir(path), "ownership.json"), &owner) != nil || owner.Version != 1 || owner.RunID != mapping.RunID || owner.Phase != "ready" || owner.ContextA != mapping.A.Context || owner.ContextB != mapping.B.Context {
		return FixtureTarget{}, denied
	}
	for _, resource := range owner.Resources {
		if resource.Context == identity.Context && resource.Kind == "ConfigMap" && resource.Name == "ks-marker" && resource.UID != "" {
			return FixtureTarget{Target: model.Target{Namespace: identity.Namespace, Name: "ks-marker"}, Cluster: identity.Context, Endpoint: identity.Server, UID: resource.UID}, nil
		}
	}
	return FixtureTarget{}, denied
}
func readFixtureJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(value); err != nil {
		return err
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return model.Fail("invalid", "Invalid fixture record")
	}
	return nil
}
