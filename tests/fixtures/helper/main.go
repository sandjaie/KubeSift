/*
Purpose: Provide a synthetic Kubernetes credential helper for local validation.
Used by: Opt-in integration and packaged helper checks.
Responsibilities: Read an ignored local fixture credential and emit short-lived ExecCredential JSON with optional failure modes.
Important: Never use provider credentials, print tokens to diagnostics or pass credentials in command-line arguments.
*/
// Synthetic helper reads only explicitly configured local fixture state.
package main

import (
	"encoding/json"
	"os"
	"time"
)

func main() {
	if os.Getenv("KUBESIFT_HELPER_MODE") == "fail" {
		os.Exit(1)
	}
	path := os.Getenv("KUBESIFT_HELPER_CREDENTIAL")
	token, err := os.ReadFile(path)
	if err != nil {
		os.Exit(1)
	}
	if counter := os.Getenv("KUBESIFT_HELPER_COUNTER"); counter != "" {
		f, e := os.OpenFile(counter, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			os.Exit(1)
		}
		_, e = f.WriteString("invocation\n")
		f.Close()
		if e != nil {
			os.Exit(1)
		}
	}
	expiry := time.Now().Add(time.Minute)
	if os.Getenv("KUBESIFT_HELPER_MODE") == "expired" {
		expiry = time.Now().Add(-time.Minute)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"apiVersion": "client.authentication.k8s.io/v1", "kind": "ExecCredential", "status": map[string]any{"token": string(token), "expirationTimestamp": expiry.UTC().Format(time.RFC3339)}})
}
