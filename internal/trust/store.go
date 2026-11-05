/*
Purpose: Remember credential-helper approvals in protected local files.
Used by: The helper trust guard and trust settings.
Responsibilities: Read a strict versioned schema and atomically save owner-only approvals with protected fingerprints.
Important: Do not persist helper arguments, environment values, tokens or certificate keys.
*/
// Package trust owns reviewed helper approvals and the actual execution boundary.
package trust

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/model"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Entry struct {
	Fingerprint string    `json:"fingerprint"`
	Label       string    `json:"label"`
	ApprovedAt  time.Time `json:"approvedAt"`
}
type record struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}
type Store struct {
	mu      sync.Mutex
	path    string
	key     []byte
	entries map[string]Entry
}

func privateRead(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, model.Fail("invalid", "Trust storage permissions must be owner-only")
	}
	return os.ReadFile(path)
}
func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, model.Fail("invalid", "Cannot open trust directory")
	}
	key, err := privateRead(path + ".key")
	if os.IsNotExist(err) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		f, e := os.OpenFile(path+".key", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return nil, model.Fail("invalid", "Cannot initialize trust key")
		}
		_, err = f.Write(key)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			return nil, model.Fail("invalid", "Cannot save trust key")
		}
	} else if err != nil {
		return nil, model.Fail("invalid", "Cannot read protected trust key")
	}
	if len(key) != 32 {
		return nil, model.Fail("invalid", "Invalid trust key")
	}
	s := &Store{path: path, key: key, entries: map[string]Entry{}}
	data, err := privateRead(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, model.Fail("invalid", "Cannot read trust records")
	}
	var r record
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&r) != nil || r.Version != 1 || len(r.Entries) > 1000 {
		return nil, model.Fail("invalid", "Invalid trust records")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, model.Fail("invalid", "Trailing trust data")
	}
	for _, e := range r.Entries {
		if len(e.Fingerprint) != 64 || e.ApprovedAt.IsZero() {
			return nil, model.Fail("invalid", "Invalid trust entry")
		}
		if _, err = hex.DecodeString(e.Fingerprint); err != nil {
			return nil, model.Fail("invalid", "Invalid trust fingerprint")
		}
		if _, exists := s.entries[e.Fingerprint]; exists {
			return nil, model.Fail("invalid", "Duplicate trust entry")
		}
		s.entries[e.Fingerprint] = e
	}
	return s, nil
}
func (s *Store) protected(fp string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(fp))
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *Store) allowedLocked(fp string) bool { _, ok := s.entries[s.protected(fp)]; return ok }
func (s *Store) saveLocked() error {
	list := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Fingerprint < list[j].Fingerprint })
	data, err := json.Marshal(record{1, list})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".trust-*")
	if err != nil {
		return model.Fail("invalid", "Cannot save trust records")
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), s.path)
	}
	if err != nil {
		return model.Fail("invalid", "Cannot save trust records")
	}
	return nil
}
func (s *Store) Approve(fp string) error { return s.approve(fp, "Credential helper") }
func (s *Store) approve(fp, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(fp) != 64 {
		return model.Fail("invalid", "Invalid fingerprint")
	}
	if _, err := hex.DecodeString(fp); err != nil {
		return model.Fail("invalid", "Invalid fingerprint")
	}
	if len(s.entries) >= 1000 {
		return model.Fail("invalid", "Trust record limit reached")
	}
	key := s.protected(fp)
	old, existed := s.entries[key]
	s.entries[key] = Entry{key, label, time.Now().UTC()}
	if err := s.saveLocked(); err != nil {
		if existed {
			s.entries[key] = old
		} else {
			delete(s.entries, key)
		}
		return err
	}
	return nil
}
func (s *Store) Revoke(fp string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.protected(fp)
	if _, ok := s.entries[fp]; ok {
		key = fp
	}
	old, ok := s.entries[key]
	delete(s.entries, key)
	if err := s.saveLocked(); err != nil {
		if ok {
			s.entries[key] = old
		}
		return err
	}
	return nil
}
func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Entry{}
	for _, e := range s.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ApprovedAt.Before(out[j].ApprovedAt) })
	return out
}
