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
require_file ".github/workflows/required-ci.yml"
require_file "mobile/eas.json"
require_file "mobile/metro.config.js"

require_text ".github/workflows/required-ci.yml" "Backend required checks"
require_text ".github/workflows/required-ci.yml" "Frontend required checks"
require_text ".github/workflows/required-ci.yml" "Mobile required checks"
require_text ".github/workflows/required-ci.yml" "npm run typecheck"
require_text ".github/workflows/required-ci.yml" "bash scripts/check-mobile-ci.sh"

require_text ".github/workflows/mobile.yml" "expo/expo-github-action@v9"
require_text ".github/workflows/mobile.yml" "eas build --platform android --profile preview"
require_text ".github/workflows/mobile.yml" "eas build --platform ios --profile ios-simulator --local"
require_text ".github/workflows/mobile.yml" "--output ../artifacts/ios/emomo-ios-simulator.tar.gz"
require_text ".github/workflows/mobile.yml" "runs-on: macos-latest"
require_text ".github/workflows/mobile.yml" "actions/upload-artifact@v6"
require_text ".github/workflows/mobile.yml" "build_ios_simulator"
require_text ".github/workflows/mobile.yml" "emomo-ios-simulator"
require_text ".github/workflows/mobile.yml" "EXPO_TOKEN"
require_text ".github/workflows/mobile.yml" "EXPO_PROJECT_ID"
require_text ".github/workflows/mobile.yml" "eas init --id"

require_text "mobile/metro.config.js" "expo/metro-config"
require_text "mobile/eas.json" '"distribution": "internal"'
require_text "mobile/eas.json" '"buildType": "apk"'
require_text "mobile/eas.json" '"ios-simulator"'
require_text "mobile/eas.json" '"simulator": true'

require_text "mobile/README.md" "EXPO_TOKEN"
require_text "mobile/README.md" "EXPO_PROJECT_ID"
require_text "mobile/README.md" "iOS simulator"
