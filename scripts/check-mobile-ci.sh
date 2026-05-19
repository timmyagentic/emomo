#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

require_file() {
  local path="$1"
  if [[ ! -f "$ROOT_DIR/$path" ]]; then
    echo "missing required file: $path" >&2
    exit 1
  fi
}

require_text() {
  local path="$1"
  local pattern="$2"
  if ! grep -Fq -- "$pattern" "$ROOT_DIR/$path"; then
    echo "missing expected text in $path: $pattern" >&2
    exit 1
  fi
}

require_file ".github/workflows/mobile.yml"
require_file "mobile/eas.json"

require_text ".github/workflows/mobile.yml" "expo/expo-github-action@v8"
require_text ".github/workflows/mobile.yml" "eas build --platform android --profile preview"
require_text ".github/workflows/mobile.yml" "actions/upload-artifact@v6"
require_text ".github/workflows/mobile.yml" "EXPO_TOKEN"

require_text "mobile/eas.json" '"distribution": "internal"'
require_text "mobile/eas.json" '"buildType": "apk"'

require_text "mobile/README.md" "EXPO_TOKEN"
