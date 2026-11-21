/*
Purpose: Define nonsecret persisted Helm source metadata.
Used by: Preferences storage and native Helm source management.
Responsibilities: Validate HTTPS/OCI addresses and optional TLS file references.
Important: Credentials and chart content never belong in this record.
*/
package preferences

import (
	"errors"
	"net/url"
	"strings"
)

type HelmSource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	URL      string `json:"url"`
	CAFile   string `json:"caFile"`
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`
}

func ValidateHelmSource(source HelmSource) error {
	if source.ID == "" || len(source.ID) > 128 || source.Name == "" || len(source.Name) > 128 {
		return errors.New("provide a bounded source ID and name")
	}
	u, err := url.Parse(source.URL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Contains(u.Path, "..") || len(source.URL) > 4096 {
		return errors.New("use a source URL without credentials, query or traversal")
	}
	if source.Kind != "https" && source.Kind != "oci" || u.Scheme != source.Kind {
		return errors.New("choose an HTTPS repository or OCI source")
	}
	if (source.CertFile == "") != (source.KeyFile == "") {
		return errors.New("select both client certificate and key")
	}
	for _, path := range []string{source.CAFile, source.CertFile, source.KeyFile} {
		if len(path) > 4096 || strings.ContainsRune(path, 0) {
			return errors.New("invalid TLS file reference")
		}
	}
	return nil
}
