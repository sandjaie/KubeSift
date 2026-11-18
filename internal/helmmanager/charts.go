/*
Purpose: Bound packaged Helm charts before invoking SDK decoders.
Used by: Native chart resolution and review snapshots.
Responsibilities: Reject unsafe archive members and bound nested dependencies before allocation.
Important: Nothing is extracted to disk and dependencies are never fetched implicitly.
*/
package helmmanager

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	"helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/loader"
)

const archiveLimit = 20 << 20
const chartExpandedLimit = 64 << 20
const reviewTTL = 5 * time.Minute

type archiveBudget struct {
	bytes   int64
	entries int
}

// Count headers and padding too: tar.Next may consume large metadata before yielding an entry.
type archiveReader struct {
	ctx    context.Context
	reader io.Reader
	budget *archiveBudget
}

func (r *archiveReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.budget.bytes <= 0 {
		return 0, model.Fail("capacity", "Chart expansion exceeds 64 MiB.")
	}
	if int64(len(p)) > r.budget.bytes {
		p = p[:r.budget.bytes]
	}
	n, err := r.reader.Read(p)
	r.budget.bytes -= int64(n)
	return n, err
}

func inspectArchive(ctx context.Context, raw []byte, depth int, budget *archiveBudget) error {
	if depth > 32 || len(raw) > archiveLimit {
		return model.Fail("capacity", "Chart archive exceeds its size or nesting limit.")
	}
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return model.Fail("invalid", "Choose a valid packaged .tgz chart.")
	}
	defer gz.Close()
	tr := tar.NewReader(&archiveReader{ctx: ctx, reader: gz, budget: budget})
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return model.Fail("invalid", "The chart archive is corrupt.")
		}
		budget.entries++
		name := strings.TrimSuffix(h.Name, "/")
		if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || path.Clean(name) != name || strings.Contains(name, "\x00") {
			return model.Fail("invalid", "Chart archive contains an unsafe path.")
		}
		for _, part := range strings.Split(name, "/") {
			if part == ".." {
				return model.Fail("invalid", "Chart archive contains traversal.")
			}
		}
		if len(strings.Split(name, "/"))+depth > 32 || budget.entries > 4096 || h.Size < 0 || h.Size > budget.bytes {
			return model.Fail("capacity", "Chart expansion exceeds 64 MiB, 4096 entries or depth 32.")
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return model.Fail("invalid", "Chart links and special files are not supported.")
		}
		if strings.HasSuffix(name, ".tgz") {
			nested, err := io.ReadAll(io.LimitReader(tr, h.Size+1))
			if err != nil {
				return model.Fail("invalid", "Cannot read embedded chart.")
			}
			if err := inspectArchive(ctx, nested, depth+1, budget); err != nil {
				return err
			}
		} else if _, err := io.Copy(io.Discard, tr); err != nil {
			return model.Fail("invalid", "Cannot read chart member.")
		}
	}
	return nil
}
func loadChart(ctx context.Context, raw []byte) (chart.Charter, error) {
	if err := inspectArchive(ctx, raw, 0, &archiveBudget{bytes: chartExpandedLimit}); err != nil {
		return nil, err
	}
	c, err := loader.LoadArchive(bytes.NewReader(raw))
	if err != nil {
		return nil, model.Fail("invalid", "Chart metadata or packaged dependencies are invalid.")
	}
	if err := completeDependencies(c); err != nil {
		return nil, err
	}
	return c, nil
}
func completeDependencies(c chart.Charter) error {
	a, err := chart.NewAccessor(c)
	if err != nil {
		return model.Fail("unsupported", "Unsupported chart format.")
	}
	present := map[string]bool{}
	for _, dep := range a.Dependencies() {
		da, err := chart.NewAccessor(dep)
		if err != nil {
			return err
		}
		present[da.Name()] = true
		if err := completeDependencies(dep); err != nil {
			return err
		}
	}
	for _, dep := range a.MetaDependencies() {
		d, err := chart.NewDependencyAccessor(dep)
		if err != nil {
			return err
		}
		if !present[d.Name()] && !present[d.Alias()] {
			return model.Fail("invalid", "A chart dependency is missing. Choose a package containing all dependencies.")
		}
	}
	return nil
}
func chartHash(raw []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) }
