#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:3001}"
PASSWORD="${SMOKE_TEST_PASSWORD:-SmokePass123!}"
VERBOSE="${SMOKE_VERBOSE:-1}"
LOG_BODY="${SMOKE_LOG_BODY:-0}"
MAX_BODY_LOG="${SMOKE_BODY_LOG_MAX:-500}"

TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t 'smoke')"
LAST_BODY_FILE="$TMP_DIR/last_body.json"
LAST_STATUS=""
LAST_BODY=""

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

log() {
  printf '[smoke] %s\n' "$1"
}

log_verbose() {
  if [[ "$VERBOSE" == "1" ]]; then
    printf '[smoke][verbose] %s\n' "$1"
  fi
}

preview_text() {
  local text="$1"
  local max_len="${2:-500}"
  local len="${#text}"
  if (( len > max_len )); then
    printf '%s... [truncated %d chars]' "${text:0:max_len}" "$((len - max_len))"
  else
    printf '%s' "$text"
  fi
}

fail() {
  printf '[smoke] ERROR: %s\n' "$1" >&2
  if [[ -f "$LAST_BODY_FILE" ]]; then
    printf '[smoke] Last response (%s): %s\n' "${LAST_STATUS:-unknown}" "$(cat "$LAST_BODY_FILE")" >&2
  fi
  exit 1
}

run_request() {
  local method="$1"
  local path="$2"
  local auth_token="${3:-}"
  local body="${4:-}"

  local url="${BASE_URL}${path}"
  local -a curl_args
  local started_at ended_at elapsed

  log_verbose "HTTP ${method} ${path}"
  if [[ -n "$body" ]]; then
    log_verbose "Payload: $(preview_text "$body" "$MAX_BODY_LOG")"
  fi

  curl_args=(
    -sS
    -X "$method"
    -o "$LAST_BODY_FILE"
    -w "%{http_code}"
    "$url"
  )

  if [[ -n "$auth_token" ]]; then
    curl_args+=( -H "Authorization: Bearer ${auth_token}" )
  fi

  if [[ -n "$body" ]]; then
    curl_args+=( -H "Content-Type: application/json" --data "$body" )
  fi

  started_at="$(date +%s)"
  LAST_STATUS="$(curl "${curl_args[@]}")"
  LAST_BODY="$(cat "$LAST_BODY_FILE")"
  ended_at="$(date +%s)"
  elapsed="$((ended_at - started_at))"

  log_verbose "Response: HTTP ${LAST_STATUS} (${elapsed}s) for ${method} ${path}"
  if [[ "$LOG_BODY" == "1" ]]; then
    log_verbose "Body: $(preview_text "$LAST_BODY" "$MAX_BODY_LOG")"
  fi
}

run_upload() {
  local file_path="$1"
  local auth_token="$2"
  local url="${BASE_URL}/api/media"
  local started_at ended_at elapsed

  log_verbose "HTTP POST /api/media (multipart upload: ${file_path})"

  started_at="$(date +%s)"
  LAST_STATUS="$(curl -sS -X POST "$url" \
    -H "Authorization: Bearer ${auth_token}" \
    -F "file=@${file_path}" \
    -o "$LAST_BODY_FILE" \
    -w "%{http_code}")"
  LAST_BODY="$(cat "$LAST_BODY_FILE")"
  ended_at="$(date +%s)"
  elapsed="$((ended_at - started_at))"

  log_verbose "Response: HTTP ${LAST_STATUS} (${elapsed}s) for POST /api/media"
  if [[ "$LOG_BODY" == "1" ]]; then
    log_verbose "Body: $(preview_text "$LAST_BODY" "$MAX_BODY_LOG")"
  fi
}

assert_status() {
  local expected="$1"
  if [[ "$LAST_STATUS" != "$expected" ]]; then
    fail "Expected HTTP ${expected}, got ${LAST_STATUS}"
  fi
}

assert_contains() {
  local text="$1"
  if [[ "$LAST_BODY" != *"$text"* ]]; then
    fail "Response does not contain expected text: $text"
  fi
}

json_extract_string() {
  local key="$1"
  local file="$2"
  sed -n "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$file" | head -n1
}

build_future_timestamp() {
  if date -u -d '+10 minutes' '+%Y-%m-%dT%H:%M:%SZ' >/dev/null 2>&1; then
    date -u -d '+10 minutes' '+%Y-%m-%dT%H:%M:%SZ'
    return
  fi

  if date -u -v+10M '+%Y-%m-%dT%H:%M:%SZ' >/dev/null 2>&1; then
    date -u -v+10M '+%Y-%m-%dT%H:%M:%SZ'
    return
  fi

  fail "Could not build a future timestamp with date command"
}

write_tiny_png() {
  local out_file="$1"
  local png_base64='iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO7ZxXQAAAAASUVORK5CYII='

  if base64 --decode >/dev/null 2>&1 <<<""; then
    printf '%s' "$png_base64" | base64 --decode > "$out_file"
  else
    printf '%s' "$png_base64" | base64 -d > "$out_file"
  fi
}

log "Starting smoke tests against ${BASE_URL}"

# 1) Health check
log "[1/7] Health check"
run_request GET /health
assert_status 200
assert_contains '"status":"healthy"'

# 2) Register and login
log "[2/7] Register and login"
TS="$(date +%s)"
EMAIL="smoke-${TS}@example.com"
NAME="Smoke Test ${TS}"

REGISTER_BODY="{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\",\"name\":\"${NAME}\"}"
run_request POST /api/auth/register "" "$REGISTER_BODY"
assert_status 201
REGISTER_TOKEN="$(json_extract_string token "$LAST_BODY_FILE")"
[[ -n "$REGISTER_TOKEN" ]] || fail "Could not extract register token"

LOGIN_BODY="{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}"
run_request POST /api/auth/login "" "$LOGIN_BODY"
assert_status 200
TOKEN="$(json_extract_string token "$LAST_BODY_FILE")"
[[ -n "$TOKEN" ]] || fail "Could not extract login token"

# 3) Auth middleware guard
log "[3/7] Auth middleware guard"
run_request GET /api/media
assert_status 401
assert_contains 'Missing authorization header'

# 4) Facebook OAuth init (OAuth-only flow)
log "[4/7] Facebook OAuth init + callback negative-path checks"
run_request GET /api/auth/facebook "$TOKEN"
if [[ "$LAST_STATUS" == "500" ]] && [[ "$LAST_BODY" == *"Facebook App ID not configured"* ]]; then
  fail "Facebook OAuth is not configured on the API server. Set FACEBOOK_APP_ID (and ideally FACEBOOK_REDIRECT_URI, FACEBOOK_APP_SECRET) in the API runtime environment."
fi
assert_status 200
assert_contains '"auth_url"'
assert_contains '"state"'
assert_contains 'facebook.com'

# 5) OAuth callback negative paths (non-interactive)
run_request GET '/auth/facebook/callback?state=abc'
assert_status 400
assert_contains 'Missing authorization code'

run_request GET '/auth/facebook/callback?code=abc'
assert_status 400
assert_contains 'Missing state parameter'

run_request GET '/auth/facebook/callback?code=abc&state=invalid-state-token'
assert_status 400
assert_contains 'Invalid or expired state token'

# 6) Media upload/list/delete
log "[5/7] Media upload/list"
PNG_FILE="$TMP_DIR/smoke.png"
write_tiny_png "$PNG_FILE"

run_upload "$PNG_FILE" "$TOKEN"
assert_status 201
MEDIA_ID="$(json_extract_string id "$LAST_BODY_FILE")"
[[ -n "$MEDIA_ID" ]] || fail "Could not extract media id from upload response"

run_request GET /api/media "$TOKEN"
assert_status 200
assert_contains "$MEDIA_ID"

# 7) Scheduled Facebook post (avoids external publish dependency)
log "[6/7] Scheduled post create/list/get"
SCHEDULED_FOR="$(build_future_timestamp)"
POST_MARKER="smoke-facebook-${TS}"
POST_BODY="{\"content\":\"${POST_MARKER}\",\"media_ids\":[\"${MEDIA_ID}\"],\"platforms\":[\"facebook\"],\"scheduled_for\":\"${SCHEDULED_FOR}\"}"

run_request POST /api/posts "$TOKEN" "$POST_BODY"
assert_status 201
assert_contains '"status":"scheduled"'
POST_ID="$(json_extract_string id "$LAST_BODY_FILE")"
[[ -n "$POST_ID" ]] || fail "Could not extract post id"

run_request GET /api/posts "$TOKEN"
assert_status 200
assert_contains "$POST_MARKER"

run_request GET "/api/posts/${POST_ID}" "$TOKEN"
assert_status 200
assert_contains "$POST_ID"

log "[7/7] Media cleanup"
run_request DELETE "/api/media/${MEDIA_ID}" "$TOKEN"
assert_status 200
assert_contains 'Media deleted successfully'

log "Smoke test completed successfully."
