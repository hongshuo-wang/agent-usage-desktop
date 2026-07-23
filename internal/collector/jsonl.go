package collector

import (
	"bufio"
	"errors"
	"io"
)

// JSONLRecord is one newline-terminated JSONL record and its file metadata.
type JSONLRecord struct {
	Data      []byte
	Offset    int64
	RawLength int64
}

// ReadJSONL reads complete JSONL records. A trailing, non-terminated record is
// left for a future scan so callers can persist the returned offset safely.
func ReadJSONL(r io.Reader, startOffset int64, visit func(JSONLRecord) error) (int64, error) {
	reader := bufio.NewReader(r)
	offset := startOffset

	for {
		physical, err := reader.ReadBytes('\n')
		if len(physical) > 0 && physical[len(physical)-1] == '\n' {
			data := physical[:len(physical)-1]
			if len(data) > 0 && data[len(data)-1] == '\r' {
				data = data[:len(data)-1]
			}

			record := JSONLRecord{
				Data:      data,
				Offset:    offset,
				RawLength: int64(len(data)),
			}
			if visitErr := visit(record); visitErr != nil {
				return offset, visitErr
			}
			offset += int64(len(physical))
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return offset, nil
			}
			return offset, err
		}
	}
}
