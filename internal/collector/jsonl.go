package collector

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

var (
	errJSONLNegativeOffset = errors.New("jsonl start offset must be non-negative")
	errJSONLOffsetOverflow = errors.New("jsonl offset overflow")
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
	if startOffset < 0 {
		return startOffset, errJSONLNegativeOffset
	}

	reader := bufio.NewReader(r)
	offset := startOffset

	for {
		physical, err := reader.ReadBytes('\n')
		if len(physical) > 0 && physical[len(physical)-1] == '\n' {
			if int64(len(physical)) > math.MaxInt64-offset {
				return offset, errJSONLOffsetOverflow
			}

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

var errJSONLSourceChanged = errors.New("jsonl source changed during scan")

type jsonlSnapshot struct {
	file     *os.File
	info     os.FileInfo
	headHash string
}

// openJSONLSnapshot captures an opened source's identity and initial head hash.
func openJSONLSnapshot(path string) (*jsonlSnapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	headHash, err := jsonlSourceHeadHash(f, info.Size())
	if err != nil {
		f.Close()
		return nil, err
	}
	pathInfo, err := os.Stat(path)
	if err != nil {
		f.Close()
		return nil, err
	}
	if !os.SameFile(info, pathInfo) {
		f.Close()
		return nil, fmt.Errorf("%w: path replaced while opening", errJSONLSourceChanged)
	}
	return &jsonlSnapshot{file: f, info: info, headHash: headHash}, nil
}

// readJSONLSnapshot indexes one immutable prefix and verifies it before writes.
func readJSONLSnapshot(path string, snapshot *jsonlSnapshot, startOffset int64, visit func(JSONLRecord) error) (int64, int64, string, error) {
	snapshotSize := snapshot.info.Size()
	if snapshotSize < startOffset {
		return startOffset, 0, "", fmt.Errorf("jsonl snapshot size %d precedes offset %d", snapshotSize, startOffset)
	}

	reader := io.NewSectionReader(snapshot.file, startOffset, snapshotSize-startOffset)
	indexedOffset, err := ReadJSONL(reader, startOffset, visit)
	if err != nil {
		return indexedOffset, 0, "", err
	}
	finalFile, err := os.Open(path)
	if err != nil {
		return indexedOffset, 0, "", err
	}
	defer finalFile.Close()
	finalInfo, err := finalFile.Stat()
	if err != nil {
		return indexedOffset, 0, "", err
	}
	if !os.SameFile(snapshot.info, finalInfo) {
		return indexedOffset, 0, "", fmt.Errorf("%w: path identity changed", errJSONLSourceChanged)
	}
	if finalInfo.Size() < snapshotSize {
		return indexedOffset, 0, "", fmt.Errorf("%w: size shrank from %d to %d", errJSONLSourceChanged, snapshotSize, finalInfo.Size())
	}
	finalHeadHash, err := jsonlSourceHeadHash(finalFile, finalInfo.Size())
	if err != nil {
		return indexedOffset, 0, "", err
	}
	initialPrefixHash, err := jsonlSourceHeadHash(finalFile, snapshotSize)
	if err != nil {
		return indexedOffset, 0, "", err
	}
	if initialPrefixHash != snapshot.headHash {
		return indexedOffset, 0, "", fmt.Errorf("%w: initial prefix changed", errJSONLSourceChanged)
	}
	pathInfo, err := os.Stat(path)
	if err != nil {
		return indexedOffset, 0, "", err
	}
	if !os.SameFile(finalInfo, pathInfo) {
		return indexedOffset, 0, "", fmt.Errorf("%w: path replaced while fingerprinting", errJSONLSourceChanged)
	}
	return indexedOffset, finalInfo.Size(), finalHeadHash, nil
}

func jsonlSourceHeadHash(f *os.File, size int64) (string, error) {
	limit := size
	if limit > 4096 {
		limit = 4096
	}
	hash := sha256.New()
	if _, err := io.CopyN(hash, io.NewSectionReader(f, 0, limit), limit); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func conciseCollectorError(err error) string {
	message := err.Error()
	if len(message) > 240 {
		return message[:240]
	}
	return message
}
