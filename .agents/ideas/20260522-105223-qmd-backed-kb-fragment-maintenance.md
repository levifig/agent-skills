---
title: QMD-backed KB fragment retrieval and maintenance
captured: 2026-05-22T10:52:23Z
status: raw
tags: [knowledge-base, qmd, markdown, retrieval, fragments, agentic-maintenance]
related:
  - 20260522-101624-sqlite-backed-operational-state.md
  - 20260522-101626-generated-markdown-review-exports.md
---

# QMD-backed KB fragment retrieval and maintenance

## Nugget

Keep knowledge-base prose canonical in Markdown, but use `tobi/qmd` or a similar local Markdown search/index layer to retrieve and reason over files, sections, and fragments. Loaf's database can track operational state and relationships; QMD can handle local KB retrieval across Markdown collections.

## Problem/Opportunity

KB maintenance should feel conversational and agentic, not like editing database records. The user may talk about a paragraph, concept, or section from a KB article and ask the model to promote, split, rewrite, or re-scope it. A Markdown-native search layer can find the relevant fragments while preserving Markdown as the source of truth.

## Initial Context

QMD describes itself as an on-device search engine for Markdown notes, meeting transcripts, documentation, and knowledge bases. It supports keyword search, vector search, hybrid query/reranking, JSON and file outputs for agents, MCP integration, context metadata, collections, and SDK access over a local SQLite index. This fits Loaf's direction: SQLite for operational state; Markdown for KB prose; QMD-style indexing for fragment retrieval and context assembly.

---

*Captured via /idea -- shape with /shape when ready*
