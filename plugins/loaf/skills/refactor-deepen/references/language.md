# Language

Shared vocabulary for every suggestion this skill makes. Use these terms exactly — don't substitute "component," "service," "API," or "boundary." Consistent language is the whole point.

> **Source:** Verbatim port from Matt Pocock's
> [improve-codebase-architecture/LANGUAGE.md](https://github.com/mattpocock/skills/blob/main/skills/engineering/improve-codebase-architecture/LANGUAGE.md).
> Loaf-context mapping notes are added inline as blockquotes labeled
> **Loaf mapping** — they annotate, never rewrite, the source.

## Contents
- Terms
- Principles
- Relationships
- Rejected framings
- Loaf mapping notes (collected)

## Terms

**Module**
Anything with an interface and an implementation. Deliberately scale-agnostic — applies equally to a function, class, package, or tier-spanning slice.
_Avoid_: unit, component, service.

> **Loaf mapping.** "Module" in this taxonomy is the source's *module* concept —
> a unit of code with a public interface and hidden implementation. It is
> **distinct from a Loaf "skill"** (a package of domain knowledge under
> `content/skills/`). When this skill talks about modules, it means the source
> definition; when Loaf talks about skills, it means the harness construct.
> Do not conflate them.

**Interface**
Everything a caller must know to use the module correctly. Includes the type signature, but also invariants, ordering constraints, error modes, required configuration, and performance characteristics.
_Avoid_: API, signature (too narrow — those refer only to the type-level surface).

> **Loaf mapping.** Loaf documentation occasionally uses "API" loosely (e.g.,
> "the CLI API"). Inside `/loaf:refactor-deepen` outputs, prefer **interface**;
> reserve "API" for the literal HTTP/CLI surface of an external system, not
> for any module's public shape.

**Implementation**
What's inside a module — its body of code. Distinct from **Adapter**: a thing can be a small adapter with a large implementation (a Postgres repo) or a large adapter with a small implementation (an in-memory fake). Reach for "adapter" when the seam is the topic; "implementation" otherwise.

**Depth**
Leverage at the interface — the amount of behaviour a caller (or test) can exercise per unit of interface they have to learn. A module is **deep** when a large amount of behaviour sits behind a small interface. A module is **shallow** when the interface is nearly as complex as the implementation.

**Seam** _(from Michael Feathers)_
A place where you can alter behaviour without editing in that place. The *location* at which a module's interface lives. Choosing where to put the seam is its own design decision, distinct from what goes behind it.
_Avoid_: boundary (overloaded with DDD's bounded context).

> **Loaf mapping.** Loaf's existing prose uses "boundary" casually in several
> places (e.g., "tool boundaries" in agent profiles). Inside this skill's
> outputs, use **seam** for the architectural construct. "Boundary" stays
> reserved for tool-access boundaries in agent profiles, not for module
> structure.

**Adapter**
A concrete thing that satisfies an interface at a seam. Describes *role* (what slot it fills), not substance (what's inside).

**Leverage**
What callers get from depth. More capability per unit of interface they have to learn. One implementation pays back across N call sites and M tests.

**Locality**
What maintainers get from depth. Change, bugs, knowledge, and verification concentrate at one place rather than spreading across callers. Fix once, fixed everywhere.

## Principles

- **Depth is a property of the interface, not the implementation.** A deep module can be internally composed of small, mockable, swappable parts — they just aren't part of the interface. A module can have **internal seams** (private to its implementation, used by its own tests) as well as the **external seam** at its interface.
- **The deletion test.** Imagine deleting the module. If complexity vanishes, the module wasn't hiding anything (it was a pass-through). If complexity reappears across N callers, the module was earning its keep.
- **The interface is the test surface.** Callers and tests cross the same seam. If you want to test *past* the interface, the module is probably the wrong shape.
- **One adapter means a hypothetical seam. Two adapters means a real one.** Don't introduce a seam unless something actually varies across it.

## Relationships

- A **Module** has exactly one **Interface** (the surface it presents to callers and tests).
- **Depth** is a property of a **Module**, measured against its **Interface**.
- A **Seam** is where a **Module**'s **Interface** lives.
- An **Adapter** sits at a **Seam** and satisfies the **Interface**.
- **Depth** produces **Leverage** for callers and **Locality** for maintainers.

## Rejected framings

- **Depth as ratio of implementation-lines to interface-lines** (Ousterhout): rewards padding the implementation. We use depth-as-leverage instead.
- **"Interface" as the TypeScript `interface` keyword or a class's public methods**: too narrow — interface here includes every fact a caller must know.
- **"Boundary"**: overloaded with DDD's bounded context. Say **seam** or **interface**.

## Loaf mapping notes (collected)

For convenience, the inline `Loaf mapping` callouts above are summarised here.

| Source term | Loaf collision | Disposition |
|---|---|---|
| **Module** | Loaf "skill" (`content/skills/{name}/`) | Different concepts; never conflate. "Module" = source definition; "skill" = Loaf harness construct. |
| **Interface** | Loaf prose says "API" loosely | Inside this skill, use **interface**. Reserve "API" for literal external HTTP/CLI surfaces. |
| **Seam** | Loaf prose says "boundary" (e.g., "tool boundaries") | Use **seam** for module structure. "Boundary" stays for tool-access boundaries in agent profiles. |

The rest of the vocabulary (Implementation, Depth, Adapter, Leverage, Locality) has no current Loaf-prose collision and is used verbatim.
