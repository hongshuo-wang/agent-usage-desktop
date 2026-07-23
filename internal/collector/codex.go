package collector

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

// CodexCollector scans Codex CLI session JSONL files and extracts usage records.
type CodexCollector struct {
	db    *storage.DB
	paths []string
}

// NewCodexCollector creates a CodexCollector that scans the given base paths.
func NewCodexCollector(db *storage.DB, paths []string) *CodexCollector {
	return &CodexCollector{db: db, paths: paths}
}

type codexEntry struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexEventPayload struct {
	Type string          `json:"type"`
	Info *codexTokenInfo `json:"info"`
}

type codexTokenInfo struct {
	LastTokenUsage *codexTokenUsage `json:"last_token_usage"`
}

type codexTokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
}

type codexPromptResponse struct {
	Role string `json:"role"`
	Type string `json:"type"`
}

// Scan walks all configured paths and processes new JSONL data from Codex CLI sessions.
func (c *CodexCollector) Scan() error {
	seenPaths := make(map[string]struct{})
	for _, basePath := range c.paths {
		err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() || filepath.Ext(path) != ".jsonl" {
				return nil
			}
			seenPaths[path] = struct{}{}
			if err := c.processFile(path); err != nil {
				log.Printf("codex: error processing %s: %v", path, err)
			}
			return nil
		})
		if err != nil {
			log.Printf("codex: cannot walk %s: %v", basePath, err)
		}
	}
	if err := c.db.MarkMissingSessionSources("codex", seenPaths); err != nil {
		return fmt.Errorf("mark missing codex sources: %w", err)
	}
	return nil
}

func (c *CodexCollector) processFile(path string) error {
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
	adapter := codexEventAdapter{}
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

	context := EventContext{Source: "codex"}
	if scanContext != nil {
		context.SessionID = scanContext.SessionID
		context.CWD = scanContext.CWD
		context.Version = scanContext.Version
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
		events, parseErr := adapter.Parse(record.Data, &context)
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

		var entry codexEntry
		if err := json.Unmarshal(record.Data, &entry); err != nil {
			return nil
		}
		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if firstTime.IsZero() && !ts.IsZero() {
			firstTime = ts
		}
		switch entry.Type {
		case "response_item":
			var response codexPromptResponse
			if err := json.Unmarshal(entry.Payload, &response); err != nil {
				return nil
			}
			if response.Type == "message" && response.Role == "user" {
				prompts++
				promptEvents = append(promptEvents, &storage.PromptEvent{Source: "codex", SessionID: context.SessionID, Timestamp: ts})
			}
		case "event_msg":
			var payload codexEventPayload
			if err := json.Unmarshal(entry.Payload, &payload); err != nil {
				return nil
			}
			if payload.Type != "token_count" || payload.Info == nil || payload.Info.LastTokenUsage == nil {
				return nil
			}
			usage := payload.Info.LastTokenUsage
			if usage.CachedInputTokens > usage.InputTokens {
				malformedLines++
				lastError = "codex token usage has cached input greater than input"
				return nil
			}
			records = append(records, &storage.UsageRecord{
				Source:                "codex",
				SessionID:             context.SessionID,
				Model:                 context.Model,
				InputTokens:           usage.InputTokens - usage.CachedInputTokens,
				OutputTokens:          usage.OutputTokens,
				CacheReadInputTokens:  usage.CachedInputTokens,
				ReasoningOutputTokens: usage.ReasoningOutputTokens,
				Timestamp:             ts,
			})
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("read codex jsonl: %w", err)
	}
	if rebuild && existingSource != nil {
		if err := c.db.DeleteSourceIndex(path); err != nil {
			return fmt.Errorf("clear codex event index: %w", err)
		}
	}
	if context.SessionID == "" {
		context.SessionID = filepath.Base(path)
		context.SessionID = context.SessionID[:len(context.SessionID)-len(filepath.Ext(context.SessionID))]
	}

	for _, record := range records {
		if record.SessionID == "" {
			record.SessionID = context.SessionID
		}
	}
	for _, event := range promptEvents {
		if event.SessionID == "" {
			event.SessionID = context.SessionID
		}
	}
	if len(records) > 0 {
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert codex usage: %w", err)
		}
	}
	if len(promptEvents) > 0 {
		if err := c.db.InsertPromptBatch(promptEvents); err != nil {
			return fmt.Errorf("insert codex prompts: %w", err)
		}
	}
	if completeRecords > 0 {
		sessionPrompts := prompts
		if rebuild && (existingSource != nil || stateSize > 0) {
			sessionPrompts = 0
		}
		if err := c.db.UpsertSession(&storage.SessionRecord{
			Source: "codex", SessionID: context.SessionID, CWD: context.CWD, Version: context.Version,
			StartTime: firstTime, Prompts: sessionPrompts,
		}); err != nil {
			return fmt.Errorf("upsert codex session: %w", err)
		}
	}
	if completeRecords == 0 && existingSource != nil && !rebuild {
		existingSource.FileSize = observedSize
		existingSource.IndexedOffset = indexedOffset
		existingSource.HeadHash = headHash
		existingSource.CoverageStatus = "partial"
		if existingSource.MalformedLines == 0 && indexedOffset == observedSize {
			existingSource.CoverageStatus = "complete"
		}
		existingSource.SourceStatus = "available"
		if _, err := c.db.UpsertSessionSource(existingSource); err != nil {
			return fmt.Errorf("update codex source: %w", err)
		}
		return c.db.SetFileState(path, observedSize, indexedOffset, scanContext)
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
		Source: "codex", SessionID: context.SessionID, SourceKind: "jsonl", Path: path,
		ParserVersion: adapter.ParserVersion(), HeadHash: headHash, FileSize: observedSize,
		IndexedOffset: indexedOffset, CoverageStatus: coverage, SourceStatus: "available",
		MalformedLines: totalMalformed, LastError: lastError, LastIndexedAt: time.Now().UTC(),
	}
	for i := range eventRecords {
		eventRecords[i].Source = "codex"
		eventRecords[i].SessionID = context.SessionID
	}
	if _, err := c.db.UpsertSessionSourceWithEvents(source, eventRecords); err != nil {
		return fmt.Errorf("upsert codex source and events: %w", err)
	}
	return c.db.SetFileState(path, observedSize, indexedOffset, &storage.FileScanContext{
		SessionID: context.SessionID, CWD: context.CWD, Version: context.Version, Model: context.Model,
	})
}
