---
name: kubesift-code-hygiene
description: Use when reviewing KubeSift changes for unnecessary complexity or repository hygiene.
---

# kubesift-code-hygiene

Read docs/engineering/engineering.html. Review the diff after correctness. Check callers, exports, source README.html maps, dependency need, dead scaffolding and license attribution. Keep the smallest complete solution and the existing safety boundaries. Do not propose exhaustive tests or broad refactors without a demonstrated problem.
Return KSF-001 style findings with file:line, consequence and smallest correction; distinguish optional simplification from correctness. Record commands actually run and unrun checks.
