import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "fs";
import { join } from "path";

import {
  checkHandler,
  isLinearNative,
  LINEAR_NATIVE_WRITE_ERROR,
  listHandler,
  proposeHandler,
  stabilizeHandler,
  upsertHandler,
} from "./kb-glossary.js";
import { readGlossary } from "../lib/kb/glossary.js";

const TEST_ROOT = join(process.cwd(), ".test-kb-glossary");

beforeEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
  mkdirSync(TEST_ROOT, { recursive: true });
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
});

function setLinearNative(enabled: boolean): void {
  const dir = join(TEST_ROOT, ".agents");
  mkdirSync(dir, { recursive: true });
  writeFileSync(
    join(dir, "loaf.json"),
    JSON.stringify({ integrations: { linear: { enabled } } }, null, 2) + "\n",
    "utf-8",
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// upsert
// ─────────────────────────────────────────────────────────────────────────────

describe("upsertHandler", () => {
  it("creates a canonical entry on first call", () => {
    const result = upsertHandler(TEST_ROOT, "Module", {
      definition: "A unit of code that hides its implementation.",
      avoid: "Service, Component",
    });
    expect(result.action).toBe("created");
    const data = readGlossary(TEST_ROOT);
    expect(data.canonical[0].name).toBe("Module");
    expect(data.canonical[0].avoid).toEqual(["Service", "Component"]);
  });

  it("updates an existing entry on second call (idempotent on identity)", () => {
    upsertHandler(TEST_ROOT, "Module", { definition: "v1" });
    const result = upsertHandler(TEST_ROOT, "Module", {
      definition: "v2",
      avoid: "Service",
    });
    expect(result.action).toBe("updated");
    const data = readGlossary(TEST_ROOT);
    expect(data.canonical).toHaveLength(1);
    expect(data.canonical[0].definition).toBe("v2");
    expect(data.canonical[0].avoid).toEqual(["Service"]);
  });

  it("treats no --avoid as an empty list (does not preserve previous aliases)", () => {
    upsertHandler(TEST_ROOT, "Module", {
      definition: "v1",
      avoid: "Service",
    });
    upsertHandler(TEST_ROOT, "Module", { definition: "v2" });
    const data = readGlossary(TEST_ROOT);
    expect(data.canonical[0].avoid).toEqual([]);
  });

  it("promotes an existing candidate when upserting same term", () => {
    proposeHandler(TEST_ROOT, "Adapter", { definition: "candidate def" });
    upsertHandler(TEST_ROOT, "Adapter", { definition: "canonical def" });
    const data = readGlossary(TEST_ROOT);
    expect(data.candidates).toHaveLength(0);
    expect(data.canonical[0].name).toBe("Adapter");
  });

  it("rejects an alias that conflicts with another canonical term", () => {
    upsertHandler(TEST_ROOT, "Module", { definition: "..." });
    const exit = vi.spyOn(process, "exit").mockImplementation((code) => {
      throw new Error(`process.exit:${code}`);
    });
    const errSpy = vi.spyOn(console, "error").mockImplementation(() => undefined);
    try {
      expect(() =>
        upsertHandler(TEST_ROOT, "Seam", {
          definition: "...",
          avoid: "Module",
        }),
      ).toThrow(/process\.exit:1/);
      const stderr = errSpy.mock.calls.map((c) => c.join(" ")).join("\n");
      expect(stderr).toMatch(/already canonical/i);
    } finally {
      exit.mockRestore();
      errSpy.mockRestore();
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// check
// ─────────────────────────────────────────────────────────────────────────────

describe("checkHandler", () => {
  it("returns canonical when term is canonical", () => {
    upsertHandler(TEST_ROOT, "Module", {
      definition: "A unit",
      avoid: "Service",
    });
    const result = checkHandler(TEST_ROOT, "Module");
    expect(result.kind).toBe("canonical");
    if (result.kind === "canonical") {
      expect(result.term.name).toBe("Module");
    }
  });

  it("returns alias when term is in an _Avoid_ list", () => {
    upsertHandler(TEST_ROOT, "Module", {
      definition: "A unit",
      avoid: "Service, Component",
    });
    const result = checkHandler(TEST_ROOT, "Service");
    expect(result.kind).toBe("alias");
    if (result.kind === "alias") {
      expect(result.canonical.name).toBe("Module");
      expect(result.alias).toBe("Service");
    }
  });

  it("returns candidate when term is in candidates", () => {
    proposeHandler(TEST_ROOT, "Adapter", { definition: "..." });
    const result = checkHandler(TEST_ROOT, "Adapter");
    expect(result.kind).toBe("candidate");
  });

  it("returns unknown for missing terms", () => {
    upsertHandler(TEST_ROOT, "Module", { definition: "..." });
    const result = checkHandler(TEST_ROOT, "Boundary");
    expect(result.kind).toBe("unknown");
  });

  it("creates the glossary lazily so check works on a fresh repo", () => {
    const result = checkHandler(TEST_ROOT, "anything");
    expect(result.kind).toBe("unknown");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// list
// ─────────────────────────────────────────────────────────────────────────────

describe("listHandler", () => {
  it("returns both sections by default (all)", () => {
    upsertHandler(TEST_ROOT, "Module", { definition: "..." });
    proposeHandler(TEST_ROOT, "Adapter", { definition: "..." });
    const result = listHandler(TEST_ROOT, "all");
    expect(result.canonical.map((t) => t.name)).toEqual(["Module"]);
    expect(result.candidates.map((t) => t.name)).toEqual(["Adapter"]);
  });

  it("filters to canonical only", () => {
    upsertHandler(TEST_ROOT, "Module", { definition: "..." });
    proposeHandler(TEST_ROOT, "Adapter", { definition: "..." });
    const result = listHandler(TEST_ROOT, "canonical");
    expect(result.candidates).toEqual([]);
    expect(result.canonical).toHaveLength(1);
  });

  it("filters to candidates only", () => {
    upsertHandler(TEST_ROOT, "Module", { definition: "..." });
    proposeHandler(TEST_ROOT, "Adapter", { definition: "..." });
    const result = listHandler(TEST_ROOT, "candidates");
    expect(result.canonical).toEqual([]);
    expect(result.candidates).toHaveLength(1);
  });

  it("returns empty arrays on a fresh glossary", () => {
    const result = listHandler(TEST_ROOT, "all");
    expect(result.canonical).toEqual([]);
    expect(result.candidates).toEqual([]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// stabilize
// ─────────────────────────────────────────────────────────────────────────────

describe("stabilizeHandler", () => {
  it("promotes a candidate to canonical and preserves its definition", () => {
    proposeHandler(TEST_ROOT, "Adapter", {
      definition: "candidate def",
      avoid: "Wrapper",
    });
    const result = stabilizeHandler(TEST_ROOT, "Adapter");
    expect(result.nextDefinition).toBe("candidate def");
    const data = readGlossary(TEST_ROOT);
    expect(data.candidates).toEqual([]);
    expect(data.canonical[0].name).toBe("Adapter");
    expect(data.canonical[0].avoid).toEqual(["Wrapper"]);
  });

  it("overrides definition when --definition is supplied", () => {
    proposeHandler(TEST_ROOT, "Adapter", { definition: "old" });
    stabilizeHandler(TEST_ROOT, "Adapter", { definition: "new" });
    const data = readGlossary(TEST_ROOT);
    expect(data.canonical[0].definition).toBe("new");
  });

  it("exits non-zero when the term is not in candidates", () => {
    const exit = vi.spyOn(process, "exit").mockImplementation((code) => {
      throw new Error(`process.exit:${code}`);
    });
    const errSpy = vi.spyOn(console, "error").mockImplementation(() => undefined);
    try {
      expect(() => stabilizeHandler(TEST_ROOT, "Nope")).toThrow(
        /process\.exit:1/,
      );
      const stderr = errSpy.mock.calls.map((c) => c.join(" ")).join("\n");
      expect(stderr).toMatch(/not in candidates/i);
    } finally {
      exit.mockRestore();
      errSpy.mockRestore();
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// propose
// ─────────────────────────────────────────────────────────────────────────────

describe("proposeHandler", () => {
  it("writes to candidates", () => {
    const result = proposeHandler(TEST_ROOT, "Adapter", {
      definition: "...",
      avoid: "Wrapper",
    });
    expect(result.action).toBe("created");
    const data = readGlossary(TEST_ROOT);
    expect(data.candidates[0].name).toBe("Adapter");
    expect(data.candidates[0].avoid).toEqual(["Wrapper"]);
  });

  it("updates an existing candidate on re-propose", () => {
    proposeHandler(TEST_ROOT, "Adapter", { definition: "v1" });
    const result = proposeHandler(TEST_ROOT, "Adapter", { definition: "v2" });
    expect(result.action).toBe("updated");
    const data = readGlossary(TEST_ROOT);
    expect(data.candidates).toHaveLength(1);
    expect(data.candidates[0].definition).toBe("v2");
  });

  it("refuses to propose a term that's already canonical", () => {
    upsertHandler(TEST_ROOT, "Module", { definition: "..." });
    const exit = vi.spyOn(process, "exit").mockImplementation((code) => {
      throw new Error(`process.exit:${code}`);
    });
    const errSpy = vi.spyOn(console, "error").mockImplementation(() => undefined);
    try {
      expect(() =>
        proposeHandler(TEST_ROOT, "Module", { definition: "..." }),
      ).toThrow(/process\.exit:1/);
      const stderr = errSpy.mock.calls.map((c) => c.join(" ")).join("\n");
      expect(stderr).toMatch(/already canonical/i);
    } finally {
      exit.mockRestore();
      errSpy.mockRestore();
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Linear-native gating
// ─────────────────────────────────────────────────────────────────────────────

describe("isLinearNative", () => {
  it("returns false when no .agents/loaf.json exists", () => {
    expect(isLinearNative(TEST_ROOT)).toBe(false);
  });

  it("returns false when integrations.linear.enabled is false", () => {
    setLinearNative(false);
    expect(isLinearNative(TEST_ROOT)).toBe(false);
  });

  it("returns true when integrations.linear.enabled is true", () => {
    setLinearNative(true);
    expect(isLinearNative(TEST_ROOT)).toBe(true);
  });

  it("re-reads the file on each call (mode toggle without restart)", () => {
    setLinearNative(false);
    expect(isLinearNative(TEST_ROOT)).toBe(false);
    setLinearNative(true);
    expect(isLinearNative(TEST_ROOT)).toBe(true);
    setLinearNative(false);
    expect(isLinearNative(TEST_ROOT)).toBe(false);
  });
});

describe("Linear-native write fail-fast", () => {
  const writeFixtures: Array<{ verb: string; run: () => void }> = [
    {
      verb: "upsert",
      run: () => upsertHandler(TEST_ROOT, "Module", { definition: "..." }),
    },
    {
      verb: "stabilize",
      run: () => stabilizeHandler(TEST_ROOT, "Adapter"),
    },
    {
      verb: "propose",
      run: () =>
        proposeHandler(TEST_ROOT, "Adapter", { definition: "..." }),
    },
  ];

  for (const { verb, run } of writeFixtures) {
    it(`${verb} writes succeed when Linear-native is disabled`, () => {
      setLinearNative(false);
      if (verb === "stabilize") {
        proposeHandler(TEST_ROOT, "Adapter", { definition: "..." });
      }
      expect(() => run()).not.toThrow();
    });

    it(`${verb} fails fast when Linear-native is enabled (gate is in handler)`, () => {
      setLinearNative(true);
      const exit = vi.spyOn(process, "exit").mockImplementation((code) => {
        throw new Error(`process.exit:${code}`);
      });
      const errSpy = vi
        .spyOn(console, "error")
        .mockImplementation(() => undefined);
      try {
        expect(() => run()).toThrow(/process\.exit:1/);
        const stderr = errSpy.mock.calls.map((c) => c.join(" ")).join("\n");
        // Spec: exact verbatim string with no prefix or decoration.
        expect(stderr).toContain(LINEAR_NATIVE_WRITE_ERROR);
      } finally {
        exit.mockRestore();
        errSpy.mockRestore();
      }
    });
  }

  it("read commands work in Linear-native mode (no gating on reads)", () => {
    setLinearNative(true);
    // No fixture file written; both reads should return empty results, not error.
    expect(() => listHandler(TEST_ROOT, "all")).not.toThrow();
    expect(() => checkHandler(TEST_ROOT, "Anything")).not.toThrow();
  });

  it("re-reads .agents/loaf.json on each handler call (toggle without restart)", () => {
    setLinearNative(true);
    expect(isLinearNative(TEST_ROOT)).toBe(true);
    setLinearNative(false);
    expect(() =>
      upsertHandler(TEST_ROOT, "Module", { definition: "..." }),
    ).not.toThrow();
  });

  it("exposes the exact fail-fast error message required by SPEC-034", () => {
    expect(LINEAR_NATIVE_WRITE_ERROR).toBe(
      "Linear-native glossary writes pending artifact-taxonomy spec — local mode only for now.",
    );
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Read-time lazy creation (regression: reads must NOT touch the filesystem)
// ─────────────────────────────────────────────────────────────────────────────

describe("read commands do not create the glossary file (lazy creation only on writes)", () => {
  it("checkHandler does not create docs/knowledge/glossary.md", () => {
    const path = join(TEST_ROOT, "docs", "knowledge", "glossary.md");
    expect(existsSync(path)).toBe(false);
    checkHandler(TEST_ROOT, "Anything");
    expect(existsSync(path)).toBe(false);
  });

  it("listHandler does not create docs/knowledge/glossary.md", () => {
    const path = join(TEST_ROOT, "docs", "knowledge", "glossary.md");
    expect(existsSync(path)).toBe(false);
    listHandler(TEST_ROOT, "all");
    expect(existsSync(path)).toBe(false);
  });

  it("upsertHandler does create the file (writes are the only creators)", () => {
    const path = join(TEST_ROOT, "docs", "knowledge", "glossary.md");
    expect(existsSync(path)).toBe(false);
    upsertHandler(TEST_ROOT, "Module", { definition: "..." });
    expect(existsSync(path)).toBe(true);
  });

  it("stabilizeHandler does NOT create the file when the term is missing (regression)", () => {
    const path = join(TEST_ROOT, "docs", "knowledge", "glossary.md");
    expect(existsSync(path)).toBe(false);
    const exit = vi.spyOn(process, "exit").mockImplementation((code) => {
      throw new Error(`process.exit:${code}`);
    });
    const errSpy = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);
    try {
      expect(() => stabilizeHandler(TEST_ROOT, "Nope")).toThrow();
      // Spec No-Go (line 102): write commands that fail must NOT leave a glossary file.
      expect(existsSync(path)).toBe(false);
    } finally {
      exit.mockRestore();
      errSpy.mockRestore();
    }
  });
});
