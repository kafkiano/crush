---
name: memory
description: Persistent memory consolidation and retrieval via cuba-memorys MCP. Activates automatically at session start (RECALL) and should be triggered at session end (CONSOLIDATE). Provides scripts to extract session data from the crush SQLite database for structured memory storage. Use when starting a new session, ending a session, or when the user asks about past work, decisions, or errors.
---

# Memory — Persistent Knowledge Across Sessions

This skill solves the "remember to remember" paradox. Instead of relying on
explicit MCP tool calls during a session, you follow a structured routine at
session boundaries. The cuba-memorys MCP backend handles storage, dedup, decay,
and retrieval. Your job is the extraction judgment.

## The Three Routines

### ROUTINE 1: RECALL (session start)

Run this at the beginning of **every** session, before responding to the user.

1. **Start a session** in cuba-memorys:
   ```
   cuba_jornada → start (name: descriptive, goals: [from user prompt])
   ```

2. **Query relevant memory** — use **specific technical terms** from the
   project, not broad topics. The search is vocabulary-sensitive:
   ```
   cuba_faro → query: "<project-name>" (matches entity names)
   cuba_faro → query: "<specific technology or pattern name>" (matches observations)
   cuba_expediente → query: "<project-name>" (known errors)
   cuba_decreto → list (returns all decisions — query is unreliable)
   ```

3. **Absorb silently** — use the retrieved context to inform your work but
   don't dump it all at the user. Mention past context only when directly
   relevant.

4. **Check pending consolidation** — read `.crush/memory/pending.jsonl` if it
   exists. If there are unprocessed sessions, process them via ROUTINE 2
   before continuing.

### ROUTINE 2: CONSOLIDATE (session end / significant milestones)

Run this when a session has produced meaningful work — a bug fixed, a decision
made, a pattern discovered, an architecture analyzed.

1. **Extract session data** using the consolidation script:
   ```bash
   bash .agents/skills/memory/scripts/consolidate.sh --last --verbose
   ```
   This reads `.crush/crush.db` and outputs the session's message history in
   a structured text format. Use `--verbose` to see truncated content previews
   for judgment. Omit `--verbose` for just tool names and line counts.

2. **Review the output** and apply the extraction guidelines from
   `references/extraction-prompt.md`. Identify:
   - **Entities** — technologies, projects, patterns discovered
   - **Facts** — non-obvious findings worth remembering
   - **Decisions** — architectural choices and rationale
   - **Errors resolved** — bugs found and fixed, with solutions
   - **Preferences** — user-stated preferences about workflow or style

3. **Store via cuba-memorys** MCP tools — follow this ordering:
   - `cuba_alma` → create/update entities first (must exist before observations)
   - `cuba_cronica` → add observations per entity (batch_add for bulk)
   - `cuba_remedio` → mark errors resolved
   - `cuba_puente` → create relations between entities
   - `cuba_decreto` → record architectural decisions

4. **Clean up duplicates** — if you discover overlapping entities from previous
   sessions, merge them: move valuable observations to the canonical entity,
   recreate relations, then `cuba_forget` the duplicate. Entity names should
   be lowercase and hyphenated.

5. **Prune superseded knowledge** — if a decision or fact is no longer accurate
   (e.g., a previous approach was abandoned), delete it with `cuba_cronica
   delete`. Stale knowledge is worse than missing knowledge.

6. **End the session** in cuba-memorys:
   ```
   cuba_jornada → end (outcome: success/partial, summary: what was accomplished)
   ```

7. **Update the consolidation marker**:
   ```bash
   mkdir -p .crush/memory && echo "$(date +%s)" > .crush/memory/last-consolidated
   ```

### ROUTINE 3: WATCH (background, optional)

For automatic session detection, run the watch script in the background:
```bash
bash .agents/skills/memory/scripts/watch.sh &
```

The watch script monitors `.crush/crush.db` for changes and writes idle session
IDs to `.crush/memory/pending.jsonl`. At the next session start, ROUTINE 1
picks these up for processing.

This is optional — you can also just run ROUTINE 2 manually at session end.

## When to Consolidate

**Always consolidate when:**
- An error was found and fixed
- An architectural decision was made
- A non-obvious project fact was discovered
- The user explicitly stated a preference

**Consolidate if significant:**
- New technologies or libraries were encountered
- Codebase patterns were discovered
- Performance characteristics were observed

**Skip:**
- Routine file reads and writes
- Obvious tool usage (grep, ls, view with no findings)
- Small talk and acknowledgements
- Anything already in memory (check before storing)

## Storage Conventions

- **Entity names**: lowercase, hyphenated (e.g., "crush", "cuba-memorys",
  "passive-consolidation"). Never camelCase or mixed case — consistency is
  critical for search retrieval.
- **Entity types**: concept, technology, project, pattern, person, config
- **Observation types**: fact, decision, lesson, preference, context,
  tool_usage
- **Relations**: uses, causes, implements, depends_on, related_to
- **Source attribution**: use "agent" for agent-discovered facts, "user" for
  user-stated preferences

## Search Behavior (Lessons Learned)

cuba-memorys uses RRF hybrid search. Understanding how it matches is essential
for effective RECALL:

- **Specific beats broad**: "SQLite timestamps seconds milliseconds" returns
  exact hits. "known errors" returns nothing.
- **Vocabulary matching**: queries must overlap with stored observation text.
  Use the same words you'd expect in the stored content.
- **Entity name queries work**: querying the entity name directly (e.g.,
  "crush") returns the entity and its neighbors via graphrag context.
- **decreto query is unreliable**: use `list` instead of `query` for decisions.
  The text matching on decisions is weak.
- **compact format**: use `format: compact` in cuba_faro to save ~35% tokens
  during routine RECALL. Use `verbose` only when you need full content.
- **graphrag_context**: search results include neighbor entities automatically.
  This helps discover related knowledge without explicit traversal.

## Anti-Patterns

- Don't store everything. Memory pollution is worse than no memory.
- Don't store things that are obvious from the code itself.
- Don't duplicate — check existing memory before storing. Run `cuba_alma get`
  for the entity first.
- Don't dump raw tool output into observations. Summarize first.
- Don't create entities for transient concepts. If it won't matter in a week,
  skip it.
- Don't leave duplicate entities. If a previous session created "Cuba-Memorys"
  and you created "cuba-memorys", merge and forget the duplicate.
- Don't leave superseded decisions. If approach A was chosen then abandoned for
  approach B, delete the decision for A.
