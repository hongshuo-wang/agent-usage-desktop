package collector

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

// PiCollector scans pi agent session JSONL files and extracts usage records.
type PiCollector struct {
	db    *storage.DB
	paths []string
}

// NewPiCollector creates a PiCollector that scans the given base paths.
func NewPiCollector(db *storage.DB, paths []string) *PiCollector {
	return &PiCollector{db: db, paths: paths}
}

type piEntry struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
	Version   int             `json:"version"`
	CWD       string          `json:"cwd"`
	ModelID   string          `json:"modelId"`
	Message   json.RawMessage `json:"message"`
}

type piMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *piUsage        `json:"usage"`
	// Tool results identify the call they answer; isError marks a failed tool.
	Timestamp  int64  `json:"timestamp"`
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	IsError    bool   `json:"isError"`
}

// piUsage reports input as non-cached input; cacheRead/cacheWrite are separate.
type piUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
	Reasoning  int64 `json:"reasoning"`
}

// Scan walks all configured paths and processes new JSONL data from pi sessions.
// Directory structure: <basePath>/<mangled-cwd>/<timestamp>_<sessionId>.jsonl
func (c *PiCollector) Scan() error {
	for _, basePath := range c.paths {
		err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || filepath.Ext(path) != ".jsonl" {
				return nil
			}
			if err := c.processFile(path, filepath.Base(filepath.Dir(path))); err != nil {
				log.Printf("pi: error processing %s: %v", path, err)
			}
			return nil
		})
		if err != nil {
			log.Printf("pi: cannot walk %s: %v", basePath, err)
		}
	}
	return nil
}

func (c *PiCollector) processFile(path, project string) error {
	snapshot, err := openJSONLSnapshot(path)
	if err != nil {
		return err
	}
	defer snapshot.file.Close()
	info := snapshot.info

	stateSize, lastOffset, scanContext, err := c.db.GetFileState(path)
	if err != nil {
		return err
	}
	adapter := piEventAdapter{}
	existingSource, err := c.db.GetSessionSourceByPath(path)
	if err != nil {
		return err
	}

	rebuild := existingSource == nil && lastOffset > 0
	if existingSource != nil {
		rebuild = rebuild || existingSource.SourceStatus != "available" ||
			existingSource.ParserVersion != adapter.ParserVersion() ||
			info.Size() < existingSource.FileSize || info.Size() < lastOffset
		if existingSource.HeadHash != "" && info.Size() >= existingSource.FileSize {
			previousHeadHash, err := jsonlSourceHeadHash(snapshot.file, existingSource.FileSize)
			if err != nil {
				return err
			}
			if existingSource.HeadHash != previousHeadHash {
				rebuild = true
			}
		}
	}
	if rebuild {
		lastOffset = 0
		scanContext = nil
	}
	if lastOffset < 0 || lastOffset > info.Size() {
		lastOffset = 0
		rebuild = true
	}

	context := EventContext{Source: "pi"}
	if scanContext != nil {
		context.SessionID = scanContext.SessionID
		context.CWD = scanContext.CWD
		context.Model = scanContext.Model
	}

	var records []*storage.UsageRecord
	var promptEvents []*storage.PromptEvent
	var eventRecords []storage.SessionEventRecord
	var prompts int
	var firstTime time.Time
	var completeRecords int
	malformedLines := 0
	lastError := ""

	indexedOffset, observedSize, headHash, err := readJSONLSnapshot(path, snapshot, lastOffset, func(record JSONLRecord) error {
		completeRecords++
		line := record.Data
		events, parseErr := adapter.Parse(line, &context)
		if parseErr != nil {
			malformedLines++
			lastError = conciseCollectorError(parseErr)
			return nil
		}
		for rawIndex, event := range events {
			eventRecords = append(eventRecords, storage.SessionEventRecord{
				EventType:       string(event.Kind),
				SourceEventType: event.SourceEventType,
				Timestamp:       event.Timestamp,
				Role:            event.Role,
				Content:         event.Content,
				ToolName:        event.ToolName,
				ToolCallID:      event.ToolCallID,
				ToolInput:       event.ToolInput,
				ToolOutput:      event.ToolOutput,
				EventStatus:     event.Status,
				DurationMS:      event.DurationMS,
				RawOffset:       record.Offset,
				RawLength:       record.RawLength,
				RawIndex:        rawIndex,
			})
		}

		var entry piEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil
		}
		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if firstTime.IsZero() && !ts.IsZero() {
			firstTime = ts
		}
		if entry.Type != "message" || len(entry.Message) == 0 {
			return nil
		}
		var msg piMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			return nil
		}
		switch msg.Role {
		case "user":
			prompts++
			promptEvents = append(promptEvents, &storage.PromptEvent{Source: "pi", Timestamp: ts})
		case "assistant":
			if msg.Usage == nil {
				return nil
			}
			records = append(records, &storage.UsageRecord{
				Source:                   "pi",
				SessionID:                context.SessionID,
				Model:                    context.Model,
				Project:                  project,
				Timestamp:                ts,
				InputTokens:              msg.Usage.Input,
				OutputTokens:             msg.Usage.Output,
				CacheReadInputTokens:     msg.Usage.CacheRead,
				CacheCreationInputTokens: msg.Usage.CacheWrite,
				ReasoningOutputTokens:    msg.Usage.Reasoning,
			})
		}
		return nil
	})
	if err != nil {
		return err
	}

	if context.SessionID == "" {
		context.SessionID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	for _, r := range records {
		if r.SessionID == "" {
			r.SessionID = context.SessionID
		}
	}
	for _, e := range promptEvents {
		if e.SessionID == "" {
			e.SessionID = context.SessionID
		}
	}
	for i := range eventRecords {
		eventRecords[i].Source = "pi"
		eventRecords[i].SessionID = context.SessionID
	}

	if len(records) > 0 {
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert pi usage: %w", err)
		}
	}
	if len(promptEvents) > 0 {
		if err := c.db.InsertPromptBatch(promptEvents); err != nil {
			return fmt.Errorf("insert pi prompts: %w", err)
		}
	}
	if prompts > 0 || len(records) > 0 {
		sessionPrompts := prompts
		if rebuild && (existingSource != nil || stateSize > 0) {
			sessionPrompts = 0
		}
		if err := c.db.UpsertSession(&storage.SessionRecord{
			Source: "pi", SessionID: context.SessionID, Project: project, CWD: context.CWD,
			StartTime: firstTime, Prompts: sessionPrompts,
		}); err != nil {
			return fmt.Errorf("upsert pi session: %w", err)
		}
	}

	totalMalformed := malformedLines
	if existingSource != nil && !rebuild {
		totalMalformed += existingSource.MalformedLines
		if lastError == "" {
			lastError = existingSource.LastError
		}
	}
	coverage := "partial"
	if totalMalformed == 0 && indexedOffset == observedSize {
		coverage = "complete"
	}
	source := &storage.SessionSource{
		Source:         "pi",
		SessionID:      context.SessionID,
		SourceKind:     "jsonl",
		Path:           path,
		ParserVersion:  adapter.ParserVersion(),
		HeadHash:       headHash,
		FileSize:       observedSize,
		IndexedOffset:  indexedOffset,
		CoverageStatus: coverage,
		SourceStatus:   "available",
		MalformedLines: totalMalformed,
		LastError:      lastError,
		LastIndexedAt:  time.Now().UTC(),
	}
	if rebuild {
		if _, err := c.db.ReplaceSessionSourceWithEvents(source, eventRecords); err != nil {
			return fmt.Errorf("replace pi source and events: %w", err)
		}
	} else if _, err := c.db.UpsertSessionSourceWithEvents(source, eventRecords); err != nil {
		return fmt.Errorf("upsert pi source and events: %w", err)
	}

	return c.db.SetFileState(path, observedSize, indexedOffset, &storage.FileScanContext{
		SessionID: context.SessionID, CWD: context.CWD, Model: context.Model,
	})
}
