import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "fs";
import { join } from "path";

import {
  ensureGlossaryExists,
  findCanonicalForAlias,
  findTerm,
  glossaryPath,
  parseGlossary,
  readGlossary,
  serializeGlossary,
  writeGlossary,
  type GlossaryData,
} from "./glossary.js";

const TEST_ROOT = join(process.cwd(), ".test-glossary");

beforeEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
  mkdirSync(TEST_ROOT, { recursive: true });
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
});

const sampleData = (): GlossaryData => ({
  frontmatter: {
    type: "glossary",
    topics: ["glossary"],
    last_reviewed: "2026-05-02",
  },
  canonical: [
    {
      name: "Module",
      definition: "A logical unit of code that hides its implementation.",
      avoid: ["Service", "Component"],
    },
    {
      name: "Seam",
      definition: "A point in code where behavior can be substituted.",
      avoid: [],
    },
  ],
  candidates: [
    {
      name: "Adapter",
      definition: "An object that translates between two interfaces.",
      avoid: ["Wrapper"],
    },
  ],
  relationships: "Modules contain Seams. Adapters bridge Modules.",
  flaggedAmbiguities: "",
});

describe("parseGlossary", () => {
  it("rejects files missing `type: glossary`", () => {
    const text = `---
topics: [foo]
---

## Canonical Terms

## Candidates

## Relationships

## Flagged ambiguities
`;
    expect(() => parseGlossary(text)).toThrow(/type: glossary/);
  });

  it("rejects files with the wrong type", () => {
    const text = `---
type: knowledge
---

## Canonical Terms

## Candidates

## Relationships

## Flagged ambiguities
`;
    expect(() => parseGlossary(text)).toThrow(/type: glossary/);
  });

  it("rejects unknown sections", () => {
    const text = `---
type: glossary
---

## Canonical Terms

## Bogus Section

## Candidates

## Relationships

## Flagged ambiguities
`;
    expect(() => parseGlossary(text)).toThrow(/unknown glossary section/);
  });

  it("rejects files missing required sections (round-trip safety)", () => {
    const text = `---
type: glossary
---

## Canonical Terms
`;
    expect(() => parseGlossary(text)).toThrow(/missing required section/);
  });

  it("rejects preamble prose before the first section header", () => {
    const text = `---
type: glossary
---

This intro paragraph would be silently dropped on rewrite.

## Canonical Terms

## Candidates

## Relationships

## Flagged ambiguities
`;
    expect(() => parseGlossary(text)).toThrow(/preamble/);
  });

  it("does not treat fenced \`## \` lines as section headers (backtick fences)", () => {
    const text = `---
type: glossary
---

## Canonical Terms

### Module

A unit of code. Example layout:

\`\`\`
## Not a real section
### Not a real term
\`\`\`

## Candidates

## Relationships

## Flagged ambiguities
`;
    const parsed = parseGlossary(text);
    expect(parsed.canonical).toHaveLength(1);
    expect(parsed.canonical[0].name).toBe("Module");
    expect(parsed.canonical[0].definition).toContain("Not a real section");
    expect(parsed.canonical[0].definition).toContain("Not a real term");
  });

  it("does not treat fenced lines as section headers (tilde fences)", () => {
    const text = `---
type: glossary
---

## Canonical Terms

### Module

A unit of code. Tilde-fenced example:

~~~
## Still not a real section
### Still not a real term
~~~

## Candidates

## Relationships

## Flagged ambiguities
`;
    const parsed = parseGlossary(text);
    expect(parsed.canonical).toHaveLength(1);
    expect(parsed.canonical[0].name).toBe("Module");
    expect(parsed.canonical[0].definition).toContain("Still not a real section");
    expect(parsed.canonical[0].definition).toContain("Still not a real term");
  });

  it("parses term definition + avoid list", () => {
    const text = `---
type: glossary
---

## Canonical Terms

### Module

A unit of code.

_Avoid_: Service, Component

## Candidates

## Relationships

## Flagged ambiguities
`;
    const parsed = parseGlossary(text);
    expect(parsed.canonical).toHaveLength(1);
    expect(parsed.canonical[0].name).toBe("Module");
    expect(parsed.canonical[0].definition).toBe("A unit of code.");
    expect(parsed.canonical[0].avoid).toEqual(["Service", "Component"]);
  });

  it("handles terms without an avoid list", () => {
    const text = `---
type: glossary
---

## Canonical Terms

### Seam

A point of substitution.

## Candidates

## Relationships

## Flagged ambiguities
`;
    const parsed = parseGlossary(text);
    expect(parsed.canonical[0].avoid).toEqual([]);
  });
});

describe("serializeGlossary + parseGlossary round-trip", () => {
  it("round-trips a populated glossary losslessly", () => {
    const data = sampleData();
    const text = serializeGlossary(data);
    const reparsed = parseGlossary(text);
    expect(serializeGlossary(reparsed)).toBe(text);
  });

  it("round-trips an empty glossary losslessly", () => {
    const data: GlossaryData = {
      frontmatter: { type: "glossary" },
      canonical: [],
      candidates: [],
      relationships: "",
      flaggedAmbiguities: "",
    };
    const text = serializeGlossary(data);
    const reparsed = parseGlossary(text);
    expect(serializeGlossary(reparsed)).toBe(text);
  });

  it("preserves relationships and flagged ambiguities prose", () => {
    const data = sampleData();
    data.flaggedAmbiguities =
      "- `Boundary` overlaps with both `Module` and `Adapter` — needs sharpening.";
    const text = serializeGlossary(data);
    const reparsed = parseGlossary(text);
    expect(reparsed.flaggedAmbiguities).toContain("Boundary");
    expect(serializeGlossary(reparsed)).toBe(text);
  });
});

describe("ensureGlossaryExists", () => {
  it("creates the file on first call and returns true", () => {
    const created = ensureGlossaryExists(TEST_ROOT);
    expect(created).toBe(true);
    expect(existsSync(glossaryPath(TEST_ROOT))).toBe(true);
  });

  it("creates parent directories lazily", () => {
    ensureGlossaryExists(TEST_ROOT);
    expect(existsSync(join(TEST_ROOT, "docs", "knowledge"))).toBe(true);
  });

  it("returns false on subsequent calls (idempotent)", () => {
    ensureGlossaryExists(TEST_ROOT);
    const second = ensureGlossaryExists(TEST_ROOT);
    expect(second).toBe(false);
  });

  it("does not overwrite existing content", () => {
    ensureGlossaryExists(TEST_ROOT);
    const data = sampleData();
    writeGlossary(TEST_ROOT, data);
    ensureGlossaryExists(TEST_ROOT);
    const reread = readGlossary(TEST_ROOT);
    expect(reread.canonical).toHaveLength(2);
  });

  it("produces a parseable empty glossary", () => {
    ensureGlossaryExists(TEST_ROOT);
    const data = readGlossary(TEST_ROOT);
    expect(data.frontmatter.type).toBe("glossary");
    expect(data.canonical).toEqual([]);
  });
});

describe("readGlossary", () => {
  it("throws when the file does not exist", () => {
    expect(() => readGlossary(TEST_ROOT)).toThrow(/not found/);
  });

  it("surfaces malformed-frontmatter errors", () => {
    const path = glossaryPath(TEST_ROOT);
    mkdirSync(join(TEST_ROOT, "docs", "knowledge"), { recursive: true });
    writeFileSync(
      path,
      `---
type: knowledge
---

## Canonical Terms

## Candidates

## Relationships

## Flagged ambiguities
`,
      "utf-8",
    );
    expect(() => readGlossary(TEST_ROOT)).toThrow(/type: glossary/);
  });

  it("reads back what was written", () => {
    writeGlossary(TEST_ROOT, sampleData());
    const data = readGlossary(TEST_ROOT);
    expect(data.canonical[0].name).toBe("Module");
    expect(data.candidates[0].name).toBe("Adapter");
  });
});

describe("writeGlossary", () => {
  it("creates parent directories if missing", () => {
    writeGlossary(TEST_ROOT, sampleData());
    expect(existsSync(glossaryPath(TEST_ROOT))).toBe(true);
  });

  it("overwrites existing content", () => {
    writeGlossary(TEST_ROOT, sampleData());
    const updated = sampleData();
    updated.canonical = [];
    writeGlossary(TEST_ROOT, updated);
    const data = readGlossary(TEST_ROOT);
    expect(data.canonical).toEqual([]);
  });

  it("ends file with a single trailing newline", () => {
    writeGlossary(TEST_ROOT, sampleData());
    const raw = readFileSync(glossaryPath(TEST_ROOT), "utf-8");
    expect(raw.endsWith("\n")).toBe(true);
    expect(raw.endsWith("\n\n\n")).toBe(false);
  });
});

describe("findTerm", () => {
  it("returns a canonical term hit with section label", () => {
    const data = sampleData();
    const found = findTerm(data, "Module");
    expect(found?.section).toBe("canonical");
    expect(found?.term.name).toBe("Module");
  });

  it("returns a candidate hit with section label", () => {
    const data = sampleData();
    const found = findTerm(data, "Adapter");
    expect(found?.section).toBe("candidates");
  });

  it("matches case-insensitively", () => {
    const data = sampleData();
    expect(findTerm(data, "module")?.term.name).toBe("Module");
    expect(findTerm(data, "MODULE")?.term.name).toBe("Module");
  });

  it("returns null for unknown terms", () => {
    const data = sampleData();
    expect(findTerm(data, "Boundary")).toBeNull();
  });
});

describe("findCanonicalForAlias", () => {
  it("locates a canonical term that lists the alias as avoided", () => {
    const data = sampleData();
    const found = findCanonicalForAlias(data, "Service");
    expect(found?.name).toBe("Module");
  });

  it("matches aliases case-insensitively", () => {
    const data = sampleData();
    expect(findCanonicalForAlias(data, "service")?.name).toBe("Module");
  });

  it("only searches canonical entries (candidates do not poison alias resolution)", () => {
    const data = sampleData();
    expect(findCanonicalForAlias(data, "Wrapper")).toBeNull();
  });

  it("returns null for unknown aliases", () => {
    const data = sampleData();
    expect(findCanonicalForAlias(data, "Foo")).toBeNull();
  });
});
