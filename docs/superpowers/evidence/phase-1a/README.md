# Phase 1A curated runtime evidence

The five files in [runtime/](runtime/) are synthetic numeric output from the completed packaged runtime profile for source revision `8b2de54`. They support the Phase 1A [validation report](../../validation/phase-1a.html), [measurement summary](../../validation/measurements.json) and [native acceptance record](native-acceptance.md):

- [renderer-measurements.jsonl](runtime/renderer-measurements.jsonl): renderer throughput, latency, queue and session samples over the measured window.
- [native-rss.jsonl](runtime/native-rss.jsonl): native process RSS samples; WebKit memory is outside this measure.
- [source-rates.jsonl](runtime/source-rates.jsonl): actual generated source rates used to validate load calibration.
- [listener-checks.json](runtime/listener-checks.json): stopped forward-port reachability checks for each cleanup cycle; session counts are in the renderer samples.
- [profile-summary.json](runtime/profile-summary.json): runner completion and flood timing.

These raw values are retained because the summary alone cannot independently establish sample coverage, load rate or cleanup bounds. Startup samples and methods are summarized in the canonical `measurements.json`; operational logs, review records, probes and handoffs remain in ignored local SDD storage.
