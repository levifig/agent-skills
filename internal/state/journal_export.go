package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// ExportKindJournal names the project-journal export.
const ExportKindJournal = "journal"

// ExportFormatJSONL names the newline-delimited JSON export format.
const ExportFormatJSONL = "jsonl"

// journalExportLimit bounds how many entries a single journal export renders.
// The timeline is project-scoped and unbounded in principle; this cap keeps a
// single export from materializing an entire project history at once.
const journalExportLimit = 5000

// ExportJournalMarkdown renders the project journal timeline as Markdown.
func ExportJournalMarkdown(ctx context.Context, root project.Root, resolver PathResolver) (MarkdownExport, error) {
	status, entries, err := journalExportEntries(ctx, root, resolver)
	if err != nil {
		return MarkdownExport{}, err
	}
	exportContext := markdownExportContextFromStatus(status, ExportAudienceLocal)
	return markdownExportResult(ExportKindJournal, exportContext, renderJournalMarkdown(exportContext, entries)), nil
}

// ExportJournalJSONL renders the project journal timeline as newline-delimited
// JSON, one entry object per line. SQLite stays canonical; JSONL is a transport
// view, never primary storage.
func ExportJournalJSONL(ctx context.Context, root project.Root, resolver PathResolver) (MarkdownExport, error) {
	status, entries, err := journalExportEntries(ctx, root, resolver)
	if err != nil {
		return MarkdownExport{}, err
	}
	exportContext := markdownExportContextFromStatus(status, ExportAudienceLocal)
	content, err := renderJournalJSONL(entries)
	if err != nil {
		return MarkdownExport{}, err
	}
	result := markdownExportResult(ExportKindJournal, exportContext, content)
	result.Format = ExportFormatJSONL
	return result, nil
}

// journalExportEntries opens the store read-only and returns the project
// timeline oldest-first (natural reading and replay order for an export).
func journalExportEntries(ctx context.Context, root project.Root, resolver PathResolver) (Status, []JournalEntryRecord, error) {
	status, err := Inspect(root, resolver)
	if err != nil {
		return Status{}, nil, err
	}
	switch status.Mode {
	case ModeMarkdownOnly:
		return Status{}, nil, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` first")
	case ModeInvalid:
		return Status{}, nil, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	store, err := OpenStoreReadOnly(status.DatabasePath)
	if err != nil {
		return Status{}, nil, fmt.Errorf("open state database for export: %w", err)
	}
	defer store.Close()

	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return Status{}, nil, err
	}
	entries, err := store.journalExportTimeline(ctx, projectID)
	if err != nil {
		return Status{}, nil, err
	}
	return status, entries, nil
}

// journalExportTimeline returns the project timeline in ascending (oldest-first)
// order for export rendering.
func (s *Store) journalExportTimeline(ctx context.Context, projectID string) ([]JournalEntryRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  id,
  entry_type,
  COALESCE(scope, ''),
  message,
  COALESCE(observed_branch, ''),
  COALESCE(observed_worktree, ''),
  COALESCE(harness_session_id, ''),
  created_at
FROM journal_entries
WHERE project_id = ?
ORDER BY created_at, rowid
LIMIT ?
`, projectID, journalExportLimit)
	if err != nil {
		return nil, fmt.Errorf("query journal export timeline: %w", err)
	}
	defer rows.Close()

	entries := []JournalEntryRecord{}
	for rows.Next() {
		var entry JournalEntryRecord
		if err := rows.Scan(
			&entry.ID,
			&entry.EntryType,
			&entry.Scope,
			&entry.Message,
			&entry.ObservedBranch,
			&entry.ObservedWorktree,
			&entry.HarnessSessionID,
			&entry.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan journal export entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal export entries: %w", err)
	}
	return entries, nil
}

func renderJournalMarkdown(ctx markdownExportContext, entries []JournalEntryRecord) string {
	var b strings.Builder
	b.WriteString("# Journal Export\n\n")
	b.WriteString("Audience: internal\n")
	b.WriteString("Source: Loaf SQLite state\n\n")
	renderMarkdownExportContext(&b, ctx)
	b.WriteString("## Journal Entries\n\n")
	if len(entries) == 0 {
		b.WriteString("No journal entries recorded.\n")
		return b.String()
	}
	for _, entry := range entries {
		label := entry.EntryType
		if entry.Scope != "" {
			label = fmt.Sprintf("%s(%s)", entry.EntryType, entry.Scope)
		}
		timestamp := formatJournalExportTimestamp(entry.CreatedAt)
		fmt.Fprintf(&b, "- [%s] `%s`: %s", timestamp, label, entry.Message)
		if entry.ObservedBranch != "" {
			fmt.Fprintf(&b, " _(branch: %s)_", entry.ObservedBranch)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderJournalJSONL(entries []JournalEntryRecord) (string, error) {
	var b strings.Builder
	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			return "", fmt.Errorf("marshal journal entry %s: %w", entry.ID, err)
		}
		b.Write(line)
		b.WriteString("\n")
	}
	return b.String(), nil
}

func formatJournalExportTimestamp(value string) string {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC().Format("2006-01-02 15:04")
	}
	if len(value) >= 16 {
		return strings.ReplaceAll(value[:16], "T", " ")
	}
	return value
}
