#!/usr/bin/env bash

# Publish local runtime config to the Cloudflare Worker/KV config center.
#
# Required:
#   CONFIG_CENTER_URL=https://worker/v1/config/emomo/production/emomo-api
#   CONFIG_CENTER_ADMIN_TOKEN=...
#
# Optional:
#   ENV_FILE=./.env
#   --dry-run

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
ENV_FILE="${ENV_FILE:-${BACKEND_DIR}/.env}"
DRY_RUN=false
ALLOW_MISSING_SECRET_REFS=false

usage() {
    cat <<EOF
Usage:
  CONFIG_CENTER_URL=... CONFIG_CENTER_ADMIN_TOKEN=... $0 [options]

Options:
  --env-file PATH    Env file to read, default: ${ENV_FILE}
  --url URL          Config center URL, overrides CONFIG_CENTER_URL
  --admin-token TOK  Admin token, overrides CONFIG_CENTER_ADMIN_TOKEN
  --allow-missing-secret-refs
                    Omit secret values that do not have a matching *_SECRET
                    binding env var. By default this is an error.
  --dry-run          Print redacted payload and do not publish
  -h, --help         Show this help

Publishes the complete effective backend config. High-sensitivity fields are
published only as Cloudflare Secrets Store binding names, never as raw values.
EOF
}

load_env_file() {
    local env_file="$1"
    local line key value

    [ -f "$env_file" ] || return 0

    while IFS= read -r line || [ -n "$line" ]; do
        line="${line#"${line%%[![:space:]]*}"}"
        line="${line%"${line##*[![:space:]]}"}"

        case "$line" in
            ""|\#*) continue ;;
        esac

        if [[ "$line" == export\ * ]]; then
            line="${line#export }"
        fi
        if [[ "$line" != *=* ]]; then
            continue
        fi

        key="${line%%=*}"
        value="${line#*=}"
        key="${key%"${key##*[![:space:]]}"}"
        key="${key#"${key%%[![:space:]]*}"}"

        if [[ ! "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
            continue
        fi
        if [ "${!key+x}" ]; then
            continue
        fi

        if [[ "$value" =~ ^\"(.*)\"$ ]]; then
            value="${BASH_REMATCH[1]}"
        elif [[ "$value" =~ ^\'(.*)\'$ ]]; then
            value="${BASH_REMATCH[1]}"
        fi

        export "$key=$value"
    done < "$env_file"
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --env-file)
            ENV_FILE="$2"
            shift 2
            ;;
        --url)
            CONFIG_CENTER_URL="$2"
            export CONFIG_CENTER_URL
            shift 2
            ;;
        --admin-token)
            CONFIG_CENTER_ADMIN_TOKEN="$2"
            export CONFIG_CENTER_ADMIN_TOKEN
            shift 2
            ;;
        --allow-missing-secret-refs)
            ALLOW_MISSING_SECRET_REFS=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown argument: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

load_env_file "$ENV_FILE"
export ENV_FILE
cd "$BACKEND_DIR"

CONFIG_CENTER_URL="${CONFIG_CENTER_URL:-}"
CONFIG_CENTER_ADMIN_TOKEN="${CONFIG_CENTER_ADMIN_TOKEN:-}"

if [ -z "$CONFIG_CENTER_URL" ]; then
    echo "CONFIG_CENTER_URL is required" >&2
    exit 1
fi

if [ "$DRY_RUN" != "true" ] && [ -z "$CONFIG_CENTER_ADMIN_TOKEN" ]; then
    echo "CONFIG_CENTER_ADMIN_TOKEN is required" >&2
    exit 1
fi

payload_file="$(mktemp)"
redacted_file="$(mktemp)"
cleanup() {
    rm -f "$payload_file" "$redacted_file"
}
trap cleanup EXIT

if [ "$ALLOW_MISSING_SECRET_REFS" = "true" ]; then
    CONFIG_CENTER_SKIP_REMOTE=true go run ./cmd/config-center-payload --allow-missing-secret-refs > "$payload_file"
else
    CONFIG_CENTER_SKIP_REMOTE=true go run ./cmd/config-center-payload > "$payload_file"
fi
cp "$payload_file" "$redacted_file"

if [ "$DRY_RUN" = "true" ]; then
    cat "$redacted_file"
    exit 0
fi

echo "Publishing runtime config to ${CONFIG_CENTER_URL}"
curl -fsS -X PUT "$CONFIG_CENTER_URL" \
    -H "Authorization: Bearer ${CONFIG_CENTER_ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    --data-binary "@${payload_file}"
echo ""
echo "Config published."
