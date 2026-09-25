#!/usr/bin/env sh
set -eu
project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"
set -a
if [ -f .env ]; then . ./.env; else . ./.env.example; fi
set +a
(command -v jq >/dev/null 2>&1) || { echo "jq is required for API validation" >&2; exit 1; }
(cd backend && go test ./... && go build ./...)
(cd frontend && npm install --no-audit --no-fund && npm run build)
docker compose config --quiet
docker compose up -d --build
cleanup() { docker compose down -v --remove-orphans; }
if [ "${KEEP_RUNNING:-0}" = "1" ]; then
  trap cleanup INT TERM
else
  trap cleanup EXIT INT TERM
fi
i=0
until curl -fsS "http://127.0.0.1:${BACKEND_PORT:-19517}/healthz" >/dev/null; do
  i=$((i+1)); [ "$i" -lt 60 ] || { docker compose logs; exit 1; }; sleep 2
done
curl -fsS "http://127.0.0.1:${FRONTEND_PORT:-18517}/" >/dev/null
login_token() {
  curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT:-19517}/api/auth/login" -H 'Content-Type: application/json' \
    -d "{\"username\":\"$1\",\"password\":\"Admin123!\"}" | jq -er '.data.token'
}
token=$(login_token admin)
viewer_token=$(login_token viewer)
operator_token=$(login_token operator)
reviewer_token=$(login_token reviewer)
[ -n "$token" ]
curl -fsS "http://127.0.0.1:${BACKEND_PORT:-19517}/api/overview" -H "Authorization: Bearer $token" >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/session" -H "Authorization: Bearer $token" | jq -e '.data.role == "admin" and (.data.requestId | length > 0)' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/runtime" -H "Authorization: Bearer $token" | jq -e '.data.appName and .data.databaseDriver and (.data.requestLimit > 0)' >/dev/null
paths=$(sed -n "s/.*path: '\\([^']*\\)'.*/\\1/p" frontend/src/types/status.ts)
for path in $paths; do
  curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/$path?page=1&pageSize=20" -H "Authorization: Bearer $token" | jq -e '.data | type == "array"' >/dev/null
done
entity_config=$(sed -n "s/.*path: '\\([^']*\\)'.*statuses: \\['\\([^']*\\)', '\\([^']*\\)'.*/\\1|\\2|\\3/p" frontend/src/types/status.ts | head -n 1)
resource=$(printf '%s' "$entity_config" | cut -d '|' -f 1)
initial_status=$(printf '%s' "$entity_config" | cut -d '|' -f 2)
next_status=$(printf '%s' "$entity_config" | cut -d '|' -f 3)
now=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
code="SMOKE-$(date +%s)"
payload=$(printf '{"code":"%s","name":"Runtime smoke record","description":"Automated Compose workflow validation","facility":"Validation Lab","owner":"admin","category":"smoke","riskLevel":"low","metricValue":1,"metricUnit":"unit","effectiveAt":"%s","evidence":"scripts/validate.sh","relatedCode":"SMOKE"}' "$code" "$now")
created=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/$resource" -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d "$payload")
id=$(printf '%s' "$created" | jq -er '.data.id')
version=$(printf '%s' "$created" | jq -er '.data.version')
printf '%s' "$created" | jq -e --arg status "$initial_status" '.data.status == $status' >/dev/null
transition=$(printf '{"status":"%s","expectedVersion":%s,"reason":"automated runtime validation"}' "$next_status" "$version")
curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/$resource/$id/transition" -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d "$transition" | jq -e --arg status "$next_status" '.data.status == $status' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/audits?page=1&pageSize=100" -H "Authorization: Bearer $token" | jq -e '.meta.total >= 2' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/audit-summary?windowHours=24" -H "Authorization: Bearer $token" | jq -e '.data.total >= 2 and .data.transitions >= 1' >/dev/null

# A viewer may inspect operations but may never mutate them or inspect audits.
viewer_write_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/runs" \
  -H "Authorization: Bearer $viewer_token" -H 'Content-Type: application/json' -d "$payload")
[ "$viewer_write_status" = "403" ]
viewer_audit_status=$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:${BACKEND_PORT}/api/audits" -H "Authorization: Bearer $viewer_token")
[ "$viewer_audit_status" = "403" ]

# Proof acceptance directly drives the linked batch. Build a batch with a
# tolerance, move it to proofing, then accept an in-tolerance and an
# over-tolerance proof and assert the batch gate state in both directions.
gate_run_code="PR-GATE-$(date +%s)"
gate_run_payload=$(printf '{"code":"%s","name":"Runtime proof-gated batch","description":"Tolerance gate validation","facility":"Validation Lab","owner":"operator","category":"calibration","riskLevel":"medium","metricValue":0.5,"metricUnit":"dE","effectiveAt":"%s","evidence":"batch colour configuration","relatedCode":"REL-GATE","colorTolerance":3.0}' "$gate_run_code" "$now")
gate_run=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/runs" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$gate_run_payload")
gate_run_id=$(printf '%s' "$gate_run" | jq -er '.data.id')
gate_run_version=$(printf '%s' "$gate_run" | jq -er '.data.version')
gate_run_path="http://127.0.0.1:${BACKEND_PORT}/api/runs/$gate_run_id/transition"
curl -fsS -X POST "$gate_run_path" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "{\"status\":\"printing\",\"expectedVersion\":$gate_run_version,\"reason\":\"plates and ink verified\"}" >/dev/null
gate_run_version=2
curl -fsS -X POST "$gate_run_path" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "{\"status\":\"proofing\",\"expectedVersion\":$gate_run_version,\"reason\":\"ready for proof\"}" >/dev/null
gate_run_version=3
# Releasing before any passed proof is blocked.
early_release_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$gate_run_path" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d "{\"status\":\"released\",\"expectedVersion\":$gate_run_version,\"reason\":\"skip proof attempt\"}")
[ "$early_release_status" = "409" ]

# First proof is over tolerance: acceptance parks the batch in hold.
bad_proof_code="CP-GATE-BAD-$(date +%s)"
bad_proof_payload=$(printf '{"code":"%s","name":"Runtime over-tolerance proof","description":"Gate fail validation","facility":"Validation Lab","owner":"operator","category":"calibration","riskLevel":"high","metricValue":4.6,"metricUnit":"dE","effectiveAt":"%s","evidence":"over limit strip","relatedCode":"REL-GATE","printRunId":%s}' "$bad_proof_code" "$now" "$gate_run_id")
bad_proof=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/proofs" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$bad_proof_payload")
bad_proof_id=$(printf '%s' "$bad_proof" | jq -er '.data.id')
bad_proof_version=$(printf '%s' "$bad_proof" | jq -er '.data.version')
bad_proof_path="http://127.0.0.1:${BACKEND_PORT}/api/proofs/$bad_proof_id/transition"
curl -fsS -X POST "$bad_proof_path" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "{\"status\":\"review\",\"expectedVersion\":$bad_proof_version,\"reason\":\"sample captured\"}" >/dev/null
bad_proof_version=2
curl -fsS -X POST "$bad_proof_path" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d "{\"status\":\"accepted\",\"expectedVersion\":$bad_proof_version,\"expectedRunVersion\":$gate_run_version,\"reason\":\"delta E over tolerance\"}" >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/runs/$gate_run_id" -H "Authorization: Bearer $reviewer_token" | jq -e '.data.status == "hold" and .data.proofVerdict == "fail" and .data.proofMeasuredDeltaE == 4.6 and .data.proofTolerance == 3.0 and .data.proofActor == "reviewer"' >/dev/null
gate_run_version=4
# A held batch cannot receive a release decision draft.
blocked_decision_code="RD-GATE-BLOCKED-$(date +%s)"
blocked_decision_payload=$(printf '{"code":"%s","name":"Blocked decision","description":"Must be rejected while held","facility":"Validation Lab","owner":"operator","category":"calibration","riskLevel":"high","metricValue":4.6,"metricUnit":"dE","effectiveAt":"%s","evidence":"held batch","relatedCode":"REL-GATE","printRunId":%s}' "$blocked_decision_code" "$now" "$gate_run_id")
blocked_decision_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/release" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$blocked_decision_payload")
[ "$blocked_decision_status" = "409" ]

# Re-proof within tolerance lifts the hold back to proofing.
good_proof_code="CP-GATE-GOOD-$(date +%s)"
good_proof_payload=$(printf '{"code":"%s","name":"Runtime passing proof","description":"Gate pass validation","facility":"Validation Lab","owner":"operator","category":"calibration","riskLevel":"low","metricValue":1.8,"metricUnit":"dE","effectiveAt":"%s","evidence":"within tolerance strip","relatedCode":"REL-GATE","printRunId":%s}' "$good_proof_code" "$now" "$gate_run_id")
good_proof=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/proofs" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$good_proof_payload")
good_proof_id=$(printf '%s' "$good_proof" | jq -er '.data.id')
good_proof_version=$(printf '%s' "$good_proof" | jq -er '.data.version')
good_proof_path="http://127.0.0.1:${BACKEND_PORT}/api/proofs/$good_proof_id/transition"
curl -fsS -X POST "$good_proof_path" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "{\"status\":\"review\",\"expectedVersion\":$good_proof_version,\"reason\":\"rework sample captured\"}" >/dev/null
good_proof_version=2
operator_accept_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$good_proof_path" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "{\"status\":\"accepted\",\"expectedVersion\":$good_proof_version,\"expectedRunVersion\":$gate_run_version,\"reason\":\"operator cannot accept\"}")
[ "$operator_accept_status" = "403" ]
# A stale batch version aborts the review and leaves the proof in review.
stale_accept_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$good_proof_path" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d "{\"status\":\"accepted\",\"expectedVersion\":$good_proof_version,\"expectedRunVersion\":3,\"reason\":\"stale batch version\"}")
[ "$stale_accept_status" = "409" ]
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/proofs/$good_proof_id" -H "Authorization: Bearer $reviewer_token" | jq -e '.data.status == "review" and .data.version == 2' >/dev/null
# Accepting with the current batch version keeps the batch in proofing/pass.
curl -fsS -X POST "$good_proof_path" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -H 'X-Request-ID: proof-accept-pass-smoke' -d "{\"status\":\"accepted\",\"expectedVersion\":$good_proof_version,\"expectedRunVersion\":$gate_run_version,\"reason\":\"colour tolerance independently verified\"}" | jq -e '.data.status == "accepted"' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/runs/$gate_run_id" -H "Authorization: Bearer $reviewer_token" | jq -e '.data.status == "proofing" and .data.proofVerdict == "pass" and .data.proofMeasuredDeltaE == 1.8' >/dev/null
gate_run_version=5

# Release decisions are versioned and only reviewer/admin may cross the release gate.
# The batch must be released (passed proof) before the decision can finalize.
curl -fsS -X POST "$gate_run_path" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d "{\"status\":\"released\",\"expectedVersion\":$gate_run_version,\"reason\":\"proof passed, release batch\"}" >/dev/null
gate_run_version=6
decision_code="RD-SMOKE-$(date +%s)"
decision_payload=$(printf '{"code":"%s","name":"Runtime release gate","description":"RBAC and immutable revision validation","facility":"Validation Lab","owner":"operator","category":"calibration","riskLevel":"medium","metricValue":1.8,"metricUnit":"dE","effectiveAt":"%s","evidence":"spectrophotometer validation evidence","relatedCode":"REL-GATE","printRunId":%s}' "$decision_code" "$now" "$gate_run_id")
decision=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/release" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -H 'X-Request-ID: release-create-smoke' -d "$decision_payload")
decision_id=$(printf '%s' "$decision" | jq -er '.data.id')
decision_version=$(printf '%s' "$decision" | jq -er '.data.version')
release_payload=$(printf '{"status":"release","expectedVersion":%s,"reason":"validated proof and colour tolerance"}' "$decision_version")
operator_release_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/release/$decision_id/transition" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$release_payload")
[ "$operator_release_status" = "403" ]
curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/release/$decision_id/transition" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -H 'X-Request-ID: release-review-smoke' -d "$release_payload" | jq -e '.data.status == "release" and .data.version == 2' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/release/$decision_id" -H "Authorization: Bearer $reviewer_token" | jq -e '.data.revisions | length == 2 and .[0].requestId == "release-review-smoke" and .[1].requestId == "release-create-smoke"' >/dev/null

docker compose ps
[ "${KEEP_RUNNING:-0}" = "1" ] && echo "KEEP_RUNNING=1: containers left running for browser validation"
