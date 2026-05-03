Delegate an implementation task to a coder sub-agent that has full access to bash, edit, write, and other tools. The coder runs autonomously in a child session and returns results when done.

<usage>
Use this tool to delegate implementation work that you've already designed and specified. The coder sub-agent can edit files, run tests, and execute commands — everything needed to implement a task from start to finish.
</usage>

<usage_notes>
1. Provide a self-contained task description that includes: what to implement, which files to modify, relevant constraints, and what tests to run. The coder has no context from your conversation — include everything it needs.
2. You can launch multiple coders concurrently by using multiple tool_use blocks in a single message. Only do this when tasks touch completely disjoint file sets (no shared files). Partition work by directory or module boundary.
3. Each coder invocation is autonomous and one-shot — it cannot communicate with you during execution. It runs, returns a result, and you review the outcome.
4. The coder sub-agent uses the same model and tools as the non-interactive coder (crush run). It has access to all MCP servers and can use the `agent` tool for its own read-only searches.
5. After the coder finishes, the tool response includes the coder's summary plus a `git diff --stat` overview of changes. Review this before continuing.
6. If the result is incorrect or incomplete, spawn another coder with a targeted fix task.
7. Do NOT use this tool for read-only searches — use the `agent` tool instead. The `coder` tool is for implementation work only.
</usage_notes>
