# Project status and evidence

This is the entry point to progress documents, not a second status tracker.

| Question | Authoritative document |
| --- | --- |
| What is current, blocked or next to execute? | [Tracked Superpowers checkpoint](../.superpowers/sdd/status.md) |
| Which implementation steps are done? | Checkboxes in the relevant [implementation plan](superpowers/plans/README.md) |
| What was tested, reviewed and integrated; what remains unverified? | The phase validation report linked from the checkpoint and plan |
| What is the intended scope, order and dependency structure? | [Roadmap](roadmap/roadmap.html) |
| Where are detailed local execution notes? | Ignored `.superpowers/sdd/` ledgers; not a portable source of completion evidence |

After verified work, update the checkpoint, applicable plan checkboxes and validation report. Keep the checkpoint concise and link to evidence instead of duplicating report histories. The checkpoint is tracked so a fresh clone or another machine can resume the work.

Update the roadmap only when scope, order, dependencies or phase exit criteria change. Do not put completed-task lists, test results, merge hashes, PR status, completion dates or moving “next/in progress/done” labels there. Documentation indexes may link to the checkpoint and evidence without becoming another progress ledger.

Historical validation reports retain their original evidence and limitations. A later status update must not silently turn an unrun check into a pass.
