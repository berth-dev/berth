## Parallel Execution Coordination

You are running in parallel with other agents. You have access to a
"coordinator" MCP server with tools for coordination. Use them:

1. **At start of work:** call `announce_intent` with your bead_id, planned files,
   and description. Check returned conflicts and adapt if overlapping.
2. **Before editing a file:** call `acquire_lock` with your bead_id and file path.
   If blocked, work on a different file first and retry later.
3. **After finishing with a file:** call `release_lock` to let other agents proceed.
4. **When making architectural decisions:** call `write_decision` with key, value,
   rationale so other agents know about structural choices.
5. **When creating new exports:** call `publish_artifact` with name, path, and
   exports list so other agents can import your work.
6. **Periodically:** call `heartbeat` with your bead_id to keep locks alive
   (locks auto-expire after 5 minutes of no heartbeat).
7. **Before reading shared state:** call `read_decisions` to see what other agents decided.

If `acquire_lock` returns blocked_by another bead, do NOT force-edit the file.
Work on other files in your bead first, then retry the lock.

Your pre-embedded Code Context section already contains KG data. Use Grep and Read
for anything not covered by the context. The coordinator tools are ONLY for
coordination with other agents, not for code understanding.
