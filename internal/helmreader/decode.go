/*
Purpose: Preflight Helm storage data before handing it to the SDK.
Used by: Request-local storage Get.
Responsibilities: Bound the base64 and gzip input and aggregate release expansion.
Important: SDK decoding starts only after bounded preflight succeeds.
*/
package helmreader

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"

	"github.com/sandjaie/KubeSift/internal/model"
)

const ExpandedLimit = 8 << 20

type decodeBudget struct{ remaining int64 }

func newBudget() *decodeBudget { return &decodeBudget{32 << 20} }

type cancelReader struct {
	context.Context
	io.Reader
}

func (r cancelReader) Read(out []byte) (int, error) {
	if err := r.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(out)
}

func (b *decodeBudget) verify(ctx context.Context, text string) error {
	_, err := b.decode(ctx, text)
	return err
}

func (b *decodeBudget) decode(ctx context.Context, text string) ([]byte, error) {
	bad := model.Fail("invalid_response", "The stored Helm release is corrupt.")
	// The API body limit bounds the encoded representation. Decode gzip from a
	// stream so an oversized release cannot enter the SDK decoder.
	input := bufio.NewReader(base64.NewDecoder(base64.StdEncoding, strings.NewReader(text)))
	header, err := input.Peek(4)
	if err != nil {
		return nil, bad
	}
	var reader io.Reader = input
	if bytes.Equal(header[:3], []byte{0x1f, 0x8b, 0x08}) {
		gzipReader, err := gzip.NewReader(input)
		if err != nil {
			return nil, bad
		}
		defer gzipReader.Close()
		reader = gzipReader
	}
	limit := min(b.remaining, ExpandedLimit)
	raw, err := io.ReadAll(io.LimitReader(cancelReader{ctx, reader}, limit+1))
	b.remaining = max(0, b.remaining-int64(len(raw)))
	if int64(len(raw)) > limit {
		return nil, model.Fail("capacity", "The Helm release exceeds the decoded size budget.")
	}
	if ctx.Err() != nil {
		return nil, model.Fail("cancelled", "The Helm read was cancelled.")
	}
	if err != nil || len(raw) == 0 || !json.Valid(raw) || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return nil, bad
	}
	return raw, nil
}
