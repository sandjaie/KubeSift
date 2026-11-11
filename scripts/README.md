# scripts

`check.sh` runs source descriptions, local documentation links, formatting, Go and frontend gates.

`check-docs.py` checks tracked HTML and Markdown file and fragment targets without HTTP requests. `--self-test` exercises the checker.

`source-descriptions.py` annotates generated bindings and checks headers.

`local-clusters.sh up` refreshes only authorized owned validation fixtures and tokens. `down` follows the ownership journal.

`measure-packaged.py --startup` launches ten sanitized packaged instances and measures frontend Ping readiness. `--profile` runs the real renderer profile, samples native RSS, and changes only the owned B fixture rate to 2000 for at least 60 seconds before restoring 100. It checks actual CRI source rates through bounded timestamped live follows, so file rotation cannot shorten the window. It uses 60-second follows and requires the predeclared ±2% over a fixed 55-second CRI interval after 1-second warm-up (at least 57 seconds full coverage), and records loopback listener checks for every post-stop cycle.

`measure-packaged-selftest.py` injects restoration and process-stop failures using synthetic objects. It never launches the app or accesses a cluster. Its generated evidence remains ignored in `.local/phase1a`.

`dev-clusters.sh` creates `dev-local-a` and `dev-local-b` when absent and applies their sample workloads. This validation does not delete those clusters.

`dev-clusters.sh --explorer` calls `explorer-fixtures.py` to collision-check and apply the bounded explorer inventory only to the existing two local clusters, without reapplying shop/billing.
