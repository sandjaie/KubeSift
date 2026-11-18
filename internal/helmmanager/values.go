/*
Purpose: Import native-selected override values into bounded ephemeral handles.
Used by: The desktop values picker and chunked editor import.
Responsibilities: Validate a mapping and share the four-draft budget with chart imports.
Important: Values handles cannot be executed or resolved as charts and expire with other drafts.
*/
package helmmanager

import (
	"context"
	"os"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
)

func (s *Service) ImportValues(path string) (string, error) {
	ctx, done := s.owned(context.Background(), 120*time.Second)
	defer done()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", model.Fail("invalid", "Choose a regular YAML values file.")
	}
	c, err := s.reserve("", 0)
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		s.Discard(c.ID)
		return "", model.Fail("unavailable", "Cannot read selected values file.")
	}
	defer file.Close()
	raw, err := readBounded(file, 1<<20)
	if err == nil {
		_, err = reviewValues(string(raw))
	}
	if err != nil {
		s.Discard(c.ID)
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || ctx.Err() != nil || s.charts[c.ID] != c {
		delete(s.charts, c.ID)
		return "", model.Fail("cancelled", "Values import cancelled.")
	}
	c.values, c.valueOffsets = string(raw), map[int]bool{0: true}
	return c.ID, nil
}

func (s *Service) ReadValues(ctx context.Context, id string, offset int) (ReviewSection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	c := s.charts[id]
	if c == nil || ctx.Err() != nil || !c.valueOffsets[offset] {
		return ReviewSection{}, model.Fail("stale_target", "Values import expired or its cursor is invalid.")
	}
	return sectionChunk(c.values, offset, c.valueOffsets)
}
