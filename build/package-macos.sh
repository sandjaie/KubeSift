#!/bin/bash
# Purpose: Assemble the local macOS validation app bundle.
# Used by: build/Taskfile.yml.
# Responsibilities: Copy the built executable, required assets and license notices, then ad-hoc sign.
# Important: Only generated bin/kubesift.app is replaced; nothing is uploaded or published.
set -euo pipefail
cd "$(dirname "$0")/.."
test -f bin/kubesift
app_bundle="bin/kubesift.app"
rm -rf -- "$app_bundle"
mkdir -p "$app_bundle/Contents/MacOS" "$app_bundle/Contents/Resources"
cp bin/kubesift "$app_bundle/Contents/MacOS/kubesift"
cp build/darwin/Info.plist "$app_bundle/Contents/Info.plist"
cp build/darwin/icons.icns "$app_bundle/Contents/Resources/"
if [ -f build/darwin/Assets.car ]; then cp build/darwin/Assets.car "$app_bundle/Contents/Resources/"; fi
cp LICENSE NOTICE "$app_bundle/Contents/Resources/"
xattr -cr "$app_bundle"
codesign --force --deep --sign - "$app_bundle"
codesign --verify --deep --strict "$app_bundle"
