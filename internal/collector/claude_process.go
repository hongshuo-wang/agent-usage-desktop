package collector

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func (c *ClaudeCollector) processFile(path, project string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	stateSize, lastOffset, scanContext, err := c.db.GetFileState(path)
	if err != nil {
		return err
	}
	adapter := claudeEventAdapter{}
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
			previousHeadHash, err := claudeSourceHeadHash(path, existingSource.FileSize)
			if err != nil {
				return err
			}
			if existingSource.HeadHash != previousHeadHash {
				rebuild = true
			}
		}
	}
	if rebuild {
		if existingSource != nil {
			if err := c.db.DeleteSourceIndex(path); err != nil {
				return fmt.Errorf("clear claude event index: %w", err)
			}
		}
		lastOffset = 0
		scanContext = nil
	}
	if lastOffset < 0 || lastOffset > info.Size() {
		lastOffset = 0
		rebuild = true
	}

	context := EventContext{Source: "claude"}
	if scanContext != nil {
		context.SessionID = scanContext.SessionID
		context.CWD = scanContext.CWD
		context.Version = scanContext.Version
		context.Model = scanContext.Model
	}
	var gitBranch string
	var records []*storage.UsageRecord
	var promptEvents []*storage.PromptEvent
	var eventRecords []storage.SessionEventRecord
	var prompts int
	var firstTime time.Time
	var completeRecords int
	malformedLines := 0
	lastError := ""

	indexedOffset, observedSize, err := readClaudeJSONLSnapshot(path, lastOffset, info.Size(), func(record JSONLRecord) error {
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

		var entry claudeEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil
		}

		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if firstTime.IsZero() && !ts.IsZero() {
			firstTime = ts
		}
		if entry.SessionID != "" {
			context.SessionID = entry.SessionID
		}
		if entry.Version != "" {
			context.Version = entry.Version
		}
		if entry.CWD != "" {
			context.CWD = entry.CWD
		}
		if entry.GitBranch != "" {
			gitBranch = entry.GitBranch
		}

		switch entry.Type {
		case "user":
			if isRealUserPrompt(entry.Message) {
				prompts++
				promptEvents = append(promptEvents, &storage.PromptEvent{
					Source: "claude", SessionID: context.SessionID, Timestamp: ts,
				})
			}
		case "assistant":
			if entry.Message == nil {
				return nil
			}
			var msg claudeMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				return nil
			}
			if msg.Usage == nil || msg.Usage.CacheCreationInputTokens == nil {
				return nil // streaming chunk, skip usage
			}
			if msg.Model == "<synthetic>" {
				return nil
			}
			if msg.Model != "" {
				context.Model = msg.Model
			}
			rec := &storage.UsageRecord{
				Source:    "claude",
				SessionID: context.SessionID,
				Model:     msg.Model,
				Timestamp: ts,
				Project:   project,
				GitBranch: gitBranch,
			}
			if msg.Usage.InputTokens != nil {
				rec.InputTokens = *msg.Usage.InputTokens
			}
			if msg.Usage.OutputTokens != nil {
				rec.OutputTokens = *msg.Usage.OutputTokens
			}
			if msg.Usage.CacheCreationInputTokens != nil {
				rec.CacheCreationInputTokens = *msg.Usage.CacheCreationInputTokens
			}
			if msg.Usage.CacheReadInputTokens != nil {
				rec.CacheReadInputTokens = *msg.Usage.CacheReadInputTokens
			}
			records = append(records, rec)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("read claude jsonl: %w", err)
	}
	headHash, err := claudeSourceHeadHash(path, observedSize)
	if err != nil {
		return err
	}

	if context.SessionID == "" {
		context.SessionID = filepath.Base(path)
		context.SessionID = context.SessionID[:len(context.SessionID)-len(filepath.Ext(context.SessionID))]
	}

	if len(records) > 0 {
		// Fill session ID for records that were parsed before we found it
		for _, r := range records {
			if r.SessionID == "" {
				r.SessionID = context.SessionID
			}
		}
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert usage: %w", err)
		}
	}

	if len(promptEvents) > 0 {
		for _, e := range promptEvents {
			if e.SessionID == "" {
				e.SessionID = context.SessionID
			}
		}
		if err := c.db.InsertPromptBatch(promptEvents); err != nil {
			return fmt.Errorf("insert prompts: %w", err)
		}
	}

	if completeRecords > 0 {
		sessionPrompts := prompts
		if rebuild && (existingSource != nil || stateSize > 0) {
			sessionPrompts = 0
		}
		sess := &storage.SessionRecord{
			Source:    "claude",
			SessionID: context.SessionID,
			Project:   project,
			CWD:       context.CWD,
			Version:   context.Version,
			GitBranch: gitBranch,
			StartTime: firstTime,
			Prompts:   sessionPrompts,
		}
		if err := c.db.UpsertSession(sess); err != nil {
			return fmt.Errorf("upsert claude session: %w", err)
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
			return fmt.Errorf("update claude source: %w", err)
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
		Source:         "claude",
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
	for i := range eventRecords {
		eventRecords[i].Source = "claude"
		eventRecords[i].SessionID = context.SessionID
	}
	if _, err := c.db.UpsertSessionSourceWithEvents(source, eventRecords); err != nil {
		return fmt.Errorf("upsert claude source and events: %w", err)
	}

	return c.db.SetFileState(path, observedSize, indexedOffset, &storage.FileScanContext{
		SessionID: context.SessionID,
		CWD:       context.CWD,
		Version:   context.Version,
		Model:     context.Model,
	})
}

func readClaudeJSONLSnapshot(path string, startOffset, snapshotSize int64, visit func(JSONLRecord) error) (int64, int64, error) {
	if snapshotSize < startOffset {
		return startOffset, 0, fmt.Errorf("claude snapshot size %d precedes offset %d", snapshotSize, startOffset)
	}
	f, err := os.Open(path)
	if err != nil {
		return startOffset, 0, err
	}
	defer f.Close()
	if startOffset > 0 {
		if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
			return startOffset, 0, err
		}
	}

	indexedOffset, err := ReadJSONL(io.LimitReader(f, snapshotSize-startOffset), startOffset, visit)
	if err != nil {
		return indexedOffset, 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return indexedOffset, 0, err
	}
	return indexedOffset, info.Size(), nil
}

func claudeSourceHeadHash(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	limit := size
	if limit > 4096 {
		limit = 4096
	}
	hash := sha256.New()
	if _, err := io.CopyN(hash, f, limit); err != nil {
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
