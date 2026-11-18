/*
Purpose: Preserve Helm scalar types and empty collections across the private renderer boundary.
Used by: The native host and trusted bundled worker.
Responsibilities: Frame bounded typed messages without changing large integers, timestamps or lookup values.
Important: Only the bundled worker exchanges this protocol; chart bytes are never decoded as gob.
*/
package wire

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"helm.sh/helm/v4/pkg/chart/common"
)

type nilMap struct{ Values bool }

type list struct {
	Items []any
	Nil   bool
}

func init() {
	gob.Register(common.Values{})
	gob.Register(map[string]any{})
	gob.Register([]any{})
	gob.Register(json.Number(""))
	gob.Register(time.Time{})
	gob.Register(list{})
	gob.Register(nilMap{})
}

// Gob preserves scalar types, but needs an explicit wrapper to distinguish empty and nil slices.
func Pack(value any) any {
	switch v := value.(type) {
	case common.Values:
		return common.Values(Pack(map[string]any(v)).(map[string]any))
	case map[string]any:
		if v == nil {
			return v
		}
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = packItem(item)
		}
		return out
	case []any:
		out := list{Items: make([]any, len(v)), Nil: v == nil}
		for i, item := range v {
			out.Items[i] = packItem(item)
		}
		return out
	default:
		return value
	}
}

func packItem(value any) any {
	switch v := value.(type) {
	case map[string]any:
		if v == nil {
			return nilMap{}
		}
	case common.Values:
		if v == nil {
			return nilMap{Values: true}
		}
	}
	return Pack(value)
}

func Unpack(value any) any {
	switch v := value.(type) {
	case nilMap:
		if v.Values {
			return common.Values(nil)
		}
		return map[string]any(nil)
	case common.Values:
		return common.Values(Unpack(map[string]any(v)).(map[string]any))
	case map[string]any:
		for key, item := range v {
			v[key] = Unpack(item)
		}
		return v
	case list:
		if v.Nil {
			return []any(nil)
		}
		out := make([]any, len(v.Items))
		for i, item := range v.Items {
			out[i] = Unpack(item)
		}
		return out
	default:
		return value
	}
}

func Write(w io.Writer, value any) error {
	var body bytes.Buffer
	if err := gob.NewEncoder(&body).Encode(value); err != nil {
		return err
	}
	if body.Len() > WireLimit {
		return fmt.Errorf("renderer message exceeds its limit")
	}
	if err := binary.Write(w, binary.BigEndian, uint32(body.Len())); err != nil {
		return err
	}
	_, err := w.Write(body.Bytes())
	return err
}

func Read(r io.Reader, value any) error {
	var size uint32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return err
	}
	if size > WireLimit {
		return fmt.Errorf("renderer message exceeds its limit")
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(r, body); err != nil {
		return err
	}
	return gob.NewDecoder(bytes.NewReader(body)).Decode(value)
}
