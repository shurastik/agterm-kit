Implement the pending tasks of the OpenSpec change `{{.Change}}`. The change name is resolved;
never re-select it.

1. Get the instructions:
   ```bash
   openspec instructions apply --change {{.Change}} --json
   ```
   It returns `contextFiles` (artifact id to file paths), progress, and the task list with status.
2. Read every file under `contextFiles` before writing any code. Use the paths the CLI returned.
3. Work through the pending tasks in order. For each one: implement it test first, run the gates
   `AGENTS.md` names for what you touched, then flip its checkbox in the tasks file (`- [ ]` to
   `- [x]`) right away. The tasks file is the durable progress record.
4. Stop when the tasks run out or the moment you hit a blocker.

When you stop, write the round report to:

{{.Report}}
