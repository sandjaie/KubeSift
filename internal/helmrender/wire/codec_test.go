/*
Purpose: Check typed renderer framing and collection fidelity at the private boundary.
Used by: Native renderer protocol tests.
Responsibilities: Preserve nil/empty collections and reject oversized or truncated frames.
Important: Synthetic payloads contain no cluster data or credentials.
*/
package wire

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"reflect"
	"testing"
)

func TestTypedFrame(t *testing.T) {
	want := map[string]any{"number": json.Number("9007199254740993"), "empty": []any{}, "nilList": []any(nil), "map": map[string]any{}, "nilMap": map[string]any(nil)}
	var frame bytes.Buffer
	if err := Write(&frame, Pack(want)); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := Read(&frame, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(Unpack(got), want) {
		t.Fatalf("typed values changed: %#v", Unpack(got))
	}
	for _, size := range []uint32{WireLimit + 1, 10} {
		frame.Reset()
		_ = binary.Write(&frame, binary.BigEndian, size)
		if Read(&frame, &got) == nil {
			t.Fatal("invalid frame accepted")
		}
	}
}
