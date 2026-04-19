# What to Consolidate

Guidelines for the agent when reviewing session data extracted by
`scripts/consolidate.sh`. Apply judgment — the script is dumb pipes, you are
the filter.

## Always Store

These are high-signal and rarely produce noise:

### Errors Resolved
- Error message (exact or close approximation)
- Root cause identified
- Solution applied
- Use `cuba_alarma` to record, `cuba_remedio` when resolved
- Tag with project name for retrieval

### Architectural Decisions
- What was chosen
- Why it was chosen (rationale)
- What alternatives were considered and rejected
- Use `cuba_decreto` — it's purpose-built for this

### Non-Obvious Project Facts
- Discovered conventions not documented in AGENTS.md
- Hidden dependencies between components
- Build/deployment quirks
- Configuration gotchas
- Use `cuba_cronica` with type "fact" attached to the project entity

### User Preferences (Explicitly Stated)
- Code style preferences the user mentioned
- Tool or workflow preferences
- Things the user said they want done a certain way
- Use `cuba_cronica` with type "preference" and source "user"

## Store If Significant

### New Technologies or Libraries
- Only if they're meaningful to the project, not just encountered in passing
- Create entity with type "technology", add facts as observations

### Codebase Patterns
- Repeated structural patterns that aren't obvious from a single file
- Cross-cutting concerns discovered through multiple tool calls
- Use `cuba_cronica` with type "context"

### Performance Characteristics
- Actual measured performance, not theoretical
- Bottlenecks identified through debugging
- Use `cuba_cronica` with type "fact"

## Skip Entirely

- Routine file reads and writes (view, edit with no surprising content)
- Tool calls that returned empty results
- Small talk, greetings, acknowledgements
- Anything already stored — check memory before storing
- Things obvious from the code itself (if a README already says it, don't
  duplicate it)
- Transient debugging steps that led nowhere
- Summary/compacted messages (already lossy)

## Entity Naming Conventions

- **Projects**: use repo or directory name (e.g., "crush", "cuba-memorys")
- **Technologies**: canonical lowercase name (e.g., "sqlite", "go", "mcp")
- **Concepts**: descriptive, hyphenated (e.g., "passive-consolidation",
  "session-lifecycle")
- **Patterns**: descriptive of the pattern (e.g., "pubsub-observer")

## Quality Check

Before storing, ask yourself:
- Would this matter if I started a fresh session tomorrow?
- Is this information not already available in project files?
- Would I search for this when working on a related task?

If the answer to all three is "no", skip it.

## Merge Checklist

After consolidation, check for duplicates and staleness:

1. **Duplicate entities**: Previous sessions may have created entities with
   different casing (e.g., "Cuba-Memorys" vs "cuba-memorys"). Merge into the
   canonical lowercase-hyphenated form:
   - Move valuable observations to the canonical entity
   - Recreate relations on the canonical entity
   - `cuba_forget` the duplicate (confirm=true)

2. **Superseded decisions**: If a previous architectural decision was abandoned
   in favor of a new approach, delete the old one with `cuba_cronica delete`.
   Keep only the current truth.

3. **Stale facts**: If an observation contradicts newer knowledge, delete the
   old one. cuba-memorys has contradiction detection (`cuba_contradiccion`) but
   explicit cleanup is more reliable.

## Storage Order

Always follow this sequence to avoid reference errors:
1. `cuba_alma` — create/update entities (must exist before observations)
2. `cuba_cronica` — add observations per entity
3. `cuba_puente` — create relations
4. `cuba_decreto` — record decisions
5. `cuba_alarma` / `cuba_remedio` — record and resolve errors
