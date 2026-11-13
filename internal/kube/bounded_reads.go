/*
Purpose: Bound decoded Kubernetes read responses before object allocation.
Used by: Workload inventory and investigation readers.
Responsibilities: Wrap the owned transport without changing credentials or stream behavior.
Important: The 16 MiB decoded-body cap applies only to reads, never log streams.
*/
package kube

import (
	"compress/gzip"
	"context"
	"github.com/sandjaie/KubeSift/internal/model"
	"io"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
	"strings"
)

const readBodyLimit = 16 << 20
const detailLimit = 2 << 20

type boundedTransport struct{ next http.RoundTripper }
type boundedBody struct {
	io.Reader
	close     func() error
	remaining int64
}

func (b *boundedBody) Close() error { return b.close() }
func (b *boundedBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.remaining == 0 {
		var extra [1]byte
		n, e := b.Reader.Read(extra[:])
		if n > 0 {
			return 0, model.Fail("capacity", "Kubernetes response exceeds the read size limit")
		}
		return 0, e
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, e := b.Reader.Read(p)
	b.remaining -= int64(n)
	return n, e
}
func (t boundedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body := response.Body
	var reader io.Reader = body
	if strings.EqualFold(response.Header.Get("Content-Encoding"), "gzip") {
		decoded, e := gzip.NewReader(body)
		if e != nil {
			body.Close()
			return nil, model.Fail("failed", "Cannot decode Kubernetes response")
		}
		reader = decoded
		response.Header.Del("Content-Encoding")
		response.ContentLength = -1
	}
	response.Body = &boundedBody{Reader: reader, close: body.Close, remaining: readBodyLimit}
	return response, nil
}
func (s *Service) boundedReadConfig(ctx context.Context, ref model.ConnectionRef) (*rest.Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, safeError(ctx, err)
	}
	cfg, err := s.connections.RESTConfig(ref)
	if err != nil {
		return nil, err
	}
	cfg = rest.CopyConfig(cfg)
	cfg.Wrap(func(next http.RoundTripper) http.RoundTripper { return boundedTransport{next} })
	return cfg, nil
}

func (s *Service) boundedReadClient(ctx context.Context, ref model.ConnectionRef) (kubernetes.Interface, error) {
	cfg, err := s.boundedReadConfig(ctx, ref)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, model.Fail("invalid", "Cannot create workload reader")
	}
	return client, nil
}
