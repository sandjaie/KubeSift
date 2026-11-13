/*
Purpose: Verify the Secret metadata transport boundary against synthetic HTTP.
Used by: Configuration reader tests.
Responsibilities: Detect unsafe content negotiation, projection and fallback.
Important: Sentinels are synthetic and must never appear in bridge evidence.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	t "time"
)

func TestSecretMetadataWire(t *testing.T) {
	for _, mode := range []string{"partial", "full", "mislabeled", "unsupported", "forbidden"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				want := "PartialObjectMetadata"
				if r.URL.Path == "/api/v1/namespaces/test/secrets" {
					want += "List"
					if r.URL.Query().Get("limit") != "100" {
						t.Error("unbounded list")
					}
				}
				if r.Method != "GET" || r.Header.Get("Accept") != "application/json;as="+want+";g=meta.k8s.io;v=v1" {
					t.Errorf("unexpected negotiation %s", r.Header.Get("Accept"))
				}
				w.Header().Set("Content-Type", "application/json")
				switch mode {
				case "unsupported":
					w.WriteHeader(406)
					fmt.Fprint(w, "SYNTHETIC-SENTINEL")
				case "forbidden":
					w.WriteHeader(403)
					fmt.Fprint(w, "SYNTHETIC-SENTINEL")
				case "full":
					if want == "PartialObjectMetadataList" {
						fmt.Fprint(w, `{"apiVersion":"v1","kind":"SecretList","items":[{"data":{"value":"SYNTHETIC-SENTINEL"}}]}`)
					} else {
						fmt.Fprint(w, `{"apiVersion":"v1","kind":"Secret","data":{"value":"SYNTHETIC-SENTINEL"}}`)
					}
				case "mislabeled":
					item := `{"metadata":{"name":"example","namespace":"test","uid":"uid"},"data":{"value":"SYNTHETIC-SENTINEL"}}`
					if want == "PartialObjectMetadataList" {
						fmt.Fprintf(w, `{"apiVersion":"meta.k8s.io/v1","kind":%q,"items":[%s]}`, want, item)
					} else {
						fmt.Fprint(w, `{"apiVersion":"meta.k8s.io/v1","kind":"PartialObjectMetadata",`+item[1:])
					}
				default:
					item := `{"metadata":{"namespace":"test","name":"example","uid":"uid","resourceVersion":"7","annotations":{"a":"SYNTHETIC-SENTINEL"}}}`
					if want == "PartialObjectMetadataList" {
						fmt.Fprintf(w, `{"apiVersion":"meta.k8s.io/v1","kind":%q,"metadata":{"continue":"next"},"items":[%s]}`, want, item)
					} else {
						fmt.Fprint(w, `{"apiVersion":"meta.k8s.io/v1","kind":"PartialObjectMetadata",`+item[1:])
					}
				}
			})
			ctx := context.Background()
			ref := ResourceRef{Connection: c, Kind: "Secret", APIVersion: "v1", Namespace: "test", Name: "example", UID: "uid"}
			if _, e := s.ReadResourceYAML(ctx, DetailRequest{Resource: ref}); e == nil {
				t.Fatal("Secret YAML allowed")
			}
			if _, e := s.ReadResourceEvents(ctx, EventRequest{Resource: ref}); e == nil {
				t.Fatal("Secret Events allowed")
			}
			if calls != 0 {
				t.Fatal("unsupported sections reached the API")
			}
			page, e := s.ReadConfiguration(ctx, PageRequest{Connection: c, Kind: "Secret", Namespace: "test"})
			if (e == nil) != (mode == "partial") || strings.Contains(fmt.Sprint(page, e), "SYNTHETIC-SENTINEL") {
				t.Fatal("incorrect list result")
			}
			detail, e := s.ReadResource(ctx, DetailRequest{Resource: ref})
			if (e == nil) != (mode == "partial") || strings.Contains(fmt.Sprint(detail, e), "SYNTHETIC-SENTINEL") {
				t.Fatal("incorrect detail result")
			}
			if mode == "partial" && (detail.ResourceVersion != "7" || len(page.Items) != 1 || page.Items[0].ObjectJSON != "{}" || page.Page.NextCursor == "") {
				t.Fatal("missing identity or continuation")
			}
			if calls != 2 {
				t.Fatal("request retried or skipped")
			}
		})
	}
}

func TestSecretMetadataBodyAndCancellation(t *testing.T) {
	t.Run("body limit", func(t *testing.T) {
		s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"apiVersion":"meta.k8s.io/v1","kind":"PartialObjectMetadataList","metadata":{},"items":[],"padding":"`)
			chunk := strings.Repeat("x", 1<<20)
			for range 17 {
				if _, err := fmt.Fprint(w, chunk); err != nil {
					return
				}
			}
			fmt.Fprint(w, `"}`)
		})
		_, err := s.ReadConfiguration(context.Background(), PageRequest{Connection: c, Kind: "Secret", Namespace: "test"})
		if err == nil || !strings.HasPrefix(err.Error(), "capacity:") {
			t.Fatal(err)
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		started := make(chan struct{})
		s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-r.Context().Done()
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan error, 1)
		go func() {
			_, err := s.ReadConfiguration(ctx, PageRequest{Connection: c, Kind: "Secret", Namespace: "test"})
			result <- err
		}()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("request did not start")
		}
		cancel()
		select {
		case err := <-result:
			if err == nil || !strings.HasPrefix(err.Error(), "cancelled:") {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("request did not cancel")
		}
	})
}
