package collector

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestJSONLReadByteAccurateRecords(t *testing.T) {
	input := []byte("{\"text\":\"中文\"}\r\n{\"id\":2}\n{\"partial\":")
	firstPhysical := []byte("{\"text\":\"中文\"}\r\n")
	secondPhysical := []byte("{\"id\":2}\n")

	var records []JSONLRecord
	next, err := ReadJSONL(bytes.NewReader(input), 0, func(record JSONLRecord) error {
		records = append(records, record)
		return nil
	})
	if err != nil {
		t.Fatalf("ReadJSONL: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}
	if got, want := records[0].Offset, int64(0); got != want {
		t.Errorf("first offset = %d, want %d", got, want)
	}
	if got, want := records[1].Offset, int64(len(firstPhysical)); got != want {
		t.Errorf("second offset = %d, want byte offset %d", got, want)
	}
	if got, want := string(records[0].Data), "{\"text\":\"中文\"}"; got != want {
		t.Errorf("first data = %q, want %q", got, want)
	}
	if got, want := records[0].RawLength, int64(len("{\"text\":\"中文\"}")); got != want {
		t.Errorf("first raw length = %d, want %d", got, want)
	}
	if got, want := string(records[1].Data), "{\"id\":2}"; got != want {
		t.Errorf("second data = %q, want %q", got, want)
	}
	if got, want := records[1].RawLength, int64(len("{\"id\":2}")); got != want {
		t.Errorf("second raw length = %d, want %d", got, want)
	}
	if got, want := next, int64(len(firstPhysical)+len(secondPhysical)); got != want {
		t.Errorf("next offset = %d, want %d", got, want)
	}
}

func TestJSONLReadLargeRecord(t *testing.T) {
	input := []byte("{\"text\":\"" + strings.Repeat("x", 11*1024*1024) + "\"}\n")
	var got JSONLRecord
	next, err := ReadJSONL(bytes.NewReader(input), 0, func(record JSONLRecord) error {
		got = record
		return nil
	})
	if err != nil {
		t.Fatalf("ReadJSONL: %v", err)
	}
	if got.RawLength != int64(len(input)-1) {
		t.Errorf("raw length = %d, want %d", got.RawLength, len(input)-1)
	}
	if next != int64(len(input)) {
		t.Errorf("next offset = %d, want %d", next, len(input))
	}
}

func TestJSONLReadAddsStartOffset(t *testing.T) {
	const startOffset int64 = 123
	input := []byte("{\"id\":1}\n{\"id\":2}\n")
	var offsets []int64
	next, err := ReadJSONL(bytes.NewReader(input), startOffset, func(record JSONLRecord) error {
		offsets = append(offsets, record.Offset)
		return nil
	})
	if err != nil {
		t.Fatalf("ReadJSONL: %v", err)
	}
	if got, want := offsets, []int64{startOffset, startOffset + int64(len("{\"id\":1}\n"))}; !equalInt64s(got, want) {
		t.Errorf("offsets = %v, want %v", got, want)
	}
	if got, want := next, startOffset+int64(len(input)); got != want {
		t.Errorf("next offset = %d, want %d", got, want)
	}
}

func TestJSONLReadCallbackErrorKeepsFailedOffset(t *testing.T) {
	input := []byte("{\"id\":1}\n{\"id\":2}\n")
	wantErr := errors.New("visitor failed")
	calls := 0
	next, err := ReadJSONL(bytes.NewReader(input), 0, func(JSONLRecord) error {
		calls++
		if calls == 2 {
			return wantErr
		}
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if got, want := next, int64(len("{\"id\":1}\n")); got != want {
		t.Errorf("next offset = %d, want failed record offset %d", got, want)
	}
}

func TestJSONLReadEmptyAndPartialInput(t *testing.T) {
	for _, input := range []string{"", "{\"partial\":"} {
		t.Run(input, func(t *testing.T) {
			const startOffset int64 = 77
			calls := 0
			next, err := ReadJSONL(strings.NewReader(input), startOffset, func(JSONLRecord) error {
				calls++
				return nil
			})
			if err != nil {
				t.Fatalf("ReadJSONL: %v", err)
			}
			if calls != 0 {
				t.Errorf("visitor calls = %d, want 0", calls)
			}
			if next != startOffset {
				t.Errorf("next offset = %d, want %d", next, startOffset)
			}
		})
	}
}

func equalInt64s(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
