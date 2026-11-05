#!/usr/bin/env python3
"""Purpose: Keep plain-English source descriptions present after binding generation.
Used by: The binding build task and scripts/check.sh.
Responsibilities: Annotate generated glue and check the required headings in project-owned Go/TS files.
Important: Handwritten descriptions stay in their source files; this script does not invent or rewrite them.
"""
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
REQUIRED = ("Purpose:", "Used by:", "Responsibilities:", "Important:")

def bindings():
    for path in (ROOT / "frontend/bindings").rglob("*.ts"):
        text = path.read_text()
        if all(heading in text[:1800] for heading in REQUIRED):
            continue
        name = path.relative_to(ROOT / "frontend/bindings").as_posix()
        if path.name == "service.ts":
            purpose = "Provide typed frontend calls to the Go desktop service."
            used = "App and the frontend bridge."
            responsibilities = "Encode explicit requests and return the service responses."
        elif path.name == "models.ts":
            purpose = "Describe JSON values exchanged with the Go service."
            used = "Generated service calls and the validation frontend."
            responsibilities = "Match Go field names, identity shapes and optional values."
        elif name.startswith("events/") or "wailsapp/wails" in name:
            purpose = "Connect generated Wails event declarations to the frontend runtime."
            used = "Wails event subscriptions and generated bindings."
            responsibilities = "Register or describe the generated event payloads."
        else:
            purpose = "Export the generated bindings for this Go package."
            used = "Frontend imports of the package bindings."
            responsibilities = "Expose generated services or model declarations from one entry point."
        header = (f"/*\nPurpose: {purpose}\nUsed by: {used}\n"
                  f"Responsibilities: {responsibilities}\n"
                  "Important: Wails generates this glue; change Go contracts and regenerate rather than editing calls here.\n*/\n")
        path.write_text(header + text)

def check():
    result = subprocess.run(["git", "ls-files", "-co", "--exclude-standard", "-z"], cwd=ROOT, check=True, capture_output=True)
    missing = []
    for name in sorted(set(result.stdout.decode().split("\0"))):
        path = ROOT / name
        if path.suffix not in (".go", ".ts", ".tsx") or not path.is_file():
            continue
        text = path.read_text()[:1800]
        absent = [heading for heading in REQUIRED if heading not in text]
        if absent:
            missing.append(f"{name}: missing {', '.join(absent)}")
    if missing:
        print("\n".join(missing))
        return 1
    print("Source descriptions: passed")
    return 0

if __name__ == "__main__":
    if len(sys.argv) != 2 or sys.argv[1] not in ("bindings", "check"):
        raise SystemExit("usage: source-descriptions.py bindings|check")
    if sys.argv[1] == "bindings":
        bindings()
    else:
        raise SystemExit(check())
