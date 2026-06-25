package state

import (
	"fmt"
	"sort"
	"strings"
)

const DurableRenderContract = "durable-doc-v1"

var durableRenderCurrentContract = DurableRenderContract

// DurableRenderField is one stable scalar field in a committed durable render.
type DurableRenderField struct {
	Key   string
	Value string
}

// DurableRenderDocument is the deterministic representation used for committed
// durable Markdown renders. It is intentionally independent of SQLite handles so
// CI can parse and re-render committed files with no state database present.
type DurableRenderDocument struct {
	Kind   string
	Fields []DurableRenderField
	Body   string
}

// DurableSpecRenderDocument converts a spec detail into the deterministic render model.
func DurableSpecRenderDocument(spec SpecDetail) DurableRenderDocument {
	fields := []DurableRenderField{
		{Key: "id", Value: firstNonEmpty(spec.Alias, spec.ID)},
		{Key: "title", Value: spec.Title},
		{Key: "status", Value: spec.Status},
	}
	if spec.Alias != "" && spec.ID != "" && spec.Alias != spec.ID {
		fields = append(fields, DurableRenderField{Key: "state_id", Value: spec.ID})
	}
	return DurableRenderDocument{
		Kind:   "spec",
		Fields: fields,
		Body:   spec.Body,
	}
}

// DurableReportRenderDocument converts a report detail into the deterministic render model.
func DurableReportRenderDocument(report ReportDetail) DurableRenderDocument {
	fields := []DurableRenderField{
		{Key: "id", Value: firstNonEmpty(report.Alias, report.ID)},
		{Key: "title", Value: report.Title},
		{Key: "status", Value: report.Status},
	}
	if report.Kind != "" {
		fields = append(fields, DurableRenderField{Key: "report_kind", Value: report.Kind})
	}
	if report.Alias != "" && report.ID != "" && report.Alias != report.ID {
		fields = append(fields, DurableRenderField{Key: "state_id", Value: report.ID})
	}
	return DurableRenderDocument{
		Kind:   "report",
		Fields: fields,
		Body:   report.Body,
	}
}

// RenderDurableDocument emits byte-stable Markdown for a durable render.
func RenderDurableDocument(doc DurableRenderDocument) (string, error) {
	return renderDurableDocumentWithContract(doc, durableRenderCurrentContract)
}

func renderDurableDocumentWithContract(doc DurableRenderDocument, contract string) (string, error) {
	kind := strings.TrimSpace(doc.Kind)
	if kind == "" {
		return "", fmt.Errorf("durable render requires kind")
	}
	contract = strings.TrimSpace(contract)
	if contract == "" {
		return "", fmt.Errorf("durable render requires contract")
	}
	fields, err := canonicalDurableFields(doc.Fields)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("---\n")
	for _, field := range fields {
		fmt.Fprintf(&b, "%s: %s\n", field.Key, renderDurableScalar(field.Value))
	}
	b.WriteString("---\n\n")
	body := normalizeDurableBody(doc.Body)
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n\n")
	}
	b.WriteString(durableRenderStamp(kind, contract))
	b.WriteString("\n")
	return b.String(), nil
}

// ParseDurableRender parses a committed durable render for self-consistency checks.
func ParseDurableRender(content string) (DurableRenderDocument, error) {
	doc, contract, err := parseDurableRenderAnyContract(content)
	if err != nil {
		return DurableRenderDocument{}, err
	}
	if contract != durableRenderCurrentContract {
		return DurableRenderDocument{}, fmt.Errorf("unsupported durable render contract %q", contract)
	}
	return doc, nil
}

func parseDurableRenderAnyContract(content string) (DurableRenderDocument, string, error) {
	normalized := normalizeLineEndings(content)
	lines := strings.Split(strings.TrimSuffix(normalized, "\n"), "\n")
	if len(lines) == 0 {
		return DurableRenderDocument{}, "", fmt.Errorf("durable render is empty")
	}
	stamp := lines[len(lines)-1]
	kind, contract, err := parseDurableRenderStamp(stamp)
	if err != nil {
		return DurableRenderDocument{}, "", err
	}
	bodyPart := strings.Join(lines[:len(lines)-1], "\n")
	fields, body, err := parseDurableFrontmatter(bodyPart)
	if err != nil {
		return DurableRenderDocument{}, "", err
	}
	return DurableRenderDocument{
		Kind:   kind,
		Fields: fields,
		Body:   strings.Trim(body, "\n"),
	}, contract, nil
}

// ReRenderDurableRender parses and re-renders content using the deterministic renderer.
func ReRenderDurableRender(content string) (string, error) {
	doc, err := ParseDurableRender(content)
	if err != nil {
		return "", err
	}
	return RenderDurableDocument(doc)
}

func durableRenderStamp(kind string, contract string) string {
	return fmt.Sprintf("<!-- loaf:render kind=%s contract=%s -->", kind, contract)
}

func parseDurableRenderStamp(line string) (string, string, error) {
	const prefix = "<!-- loaf:render "
	const suffix = " -->"
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) {
		return "", "", fmt.Errorf("durable render missing loaf render stamp")
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix)
	values := map[string]string{}
	for _, part := range strings.Fields(inner) {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" || value == "" {
			return "", "", fmt.Errorf("invalid durable render stamp field %q", part)
		}
		values[key] = value
	}
	kind := values["kind"]
	contract := values["contract"]
	if kind == "" || contract == "" {
		return "", "", fmt.Errorf("durable render stamp requires kind and contract")
	}
	return kind, contract, nil
}

func parseDurableFrontmatter(content string) ([]DurableRenderField, string, error) {
	content = normalizeLineEndings(content)
	if !strings.HasPrefix(content, "---\n") {
		return nil, "", fmt.Errorf("durable render missing frontmatter")
	}
	rest := strings.TrimPrefix(content, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, "", fmt.Errorf("durable render frontmatter is not closed")
	}
	rawFields := rest[:end]
	body := strings.TrimPrefix(rest[end+len("\n---"):], "\n")
	fields := []DurableRenderField{}
	seen := map[string]bool{}
	for _, line := range strings.Split(rawFields, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, "", fmt.Errorf("invalid durable render frontmatter line %q", line)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, "", fmt.Errorf("durable render frontmatter field has empty key")
		}
		if seen[key] {
			return nil, "", fmt.Errorf("duplicate durable render frontmatter field %q", key)
		}
		seen[key] = true
		fields = append(fields, DurableRenderField{Key: key, Value: parseDurableScalar(strings.TrimSpace(value))})
	}
	return fields, body, nil
}

func canonicalDurableFields(fields []DurableRenderField) ([]DurableRenderField, error) {
	result := make([]DurableRenderField, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		key := strings.TrimSpace(field.Key)
		if key == "" {
			return nil, fmt.Errorf("durable render field has empty key")
		}
		if strings.ContainsAny(key, ":\n\r\t ") {
			return nil, fmt.Errorf("invalid durable render field key %q", key)
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate durable render field %q", key)
		}
		seen[key] = true
		result = append(result, DurableRenderField{Key: key, Value: normalizeDurableScalar(field.Value)})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Key < result[j].Key
	})
	return result, nil
}

func normalizeDurableBody(body string) string {
	return strings.Trim(normalizeLineEndings(body), "\n")
}

func normalizeDurableScalar(value string) string {
	return strings.TrimSpace(normalizeLineEndings(value))
}

func renderDurableScalar(value string) string {
	value = normalizeDurableScalar(value)
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, "\n\r") {
		return strings.ReplaceAll(value, "\n", " ")
	}
	if strings.ContainsAny(value, `:#"'`) || strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
	}
	return value
}

func parseDurableScalar(value string) string {
	value = strings.TrimSpace(value)
	if value == `""` {
		return ""
	}
	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		unquoted := strings.TrimSuffix(strings.TrimPrefix(value, `"`), `"`)
		unquoted = strings.ReplaceAll(unquoted, `\"`, `"`)
		unquoted = strings.ReplaceAll(unquoted, `\\`, `\`)
		return unquoted
	}
	return value
}

func normalizeLineEndings(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\r", "\n")
}
