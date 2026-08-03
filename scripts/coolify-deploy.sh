#!/usr/bin/env bash
# Trigger a Coolify deployment and wait for it to finish.
#
# Kept as a script rather than inline in the workflow because the first version
# was inline, and an embedded Python heredoc broke the YAML — the workflow was
# not even parseable. A file can be run and tested; a heredoc inside a YAML
# string inside a shell block cannot.
#
#   COOLIFY_API_URL         https://cp.nabuxai.com/api/v1
#   COOLIFY_API_TOKEN       a token scoped to deploy this resource
#   COOLIFY_RESOURCE_UUID   the resource to deploy
set -uo pipefail

for v in COOLIFY_API_URL COOLIFY_API_TOKEN COOLIFY_RESOURCE_UUID; do
  if [ -z "${!v:-}" ]; then
    echo "::error::$v is not set"
    exit 1
  fi
done

API="${COOLIFY_API_URL%/}"

# --fail-with-body: a bare curl exits 0 on a 401, and a deployment that never
# started would be reported as a success.
response="$(curl --fail-with-body --silent --show-error \
  --header "Authorization: Bearer ${COOLIFY_API_TOKEN}" \
  "${API}/deploy?uuid=${COOLIFY_RESOURCE_UUID}&force=false")" || {
    echo "::error::Coolify refused the deployment request"
    echo "$response"
    exit 1
  }
echo "$response"

uuid="$(printf '%s' "$response" | python3 -c 'import json,sys
try: body = json.load(sys.stdin)
except Exception: sys.exit(0)
d = body.get("deployments") or []
print(d[0].get("deployment_uuid","") if d else "")')"

if [ -z "$uuid" ]; then
  echo "::error::Coolify accepted the request but named no deployment to watch."
  echo "Without an id this cannot tell a finished deploy from a failed one, and"
  echo "reporting success would be a guess."
  exit 1
fi
echo "watching deployment $uuid"

# Firing the webhook and exiting is the version of this that is always green:
# it reports that a request was sent, which is not a build that succeeded and a
# container that started.
for attempt in $(seq 1 90); do
  body="$(curl --silent --show-error \
    --header "Authorization: Bearer ${COOLIFY_API_TOKEN}" \
    "${API}/deployments/${uuid}" || true)"

  status="$(printf '%s' "$body" | python3 -c 'import json,sys
try: print(json.load(sys.stdin).get("status",""))
except Exception: print("")')"

  echo "[$attempt] status: ${status:-unknown}"
  case "$status" in
    finished|success)
      echo "deployment finished"
      exit 0 ;;
    failed|cancelled|error)
      echo "::error::Deployment $status. The previous build is still serving, so"
      echo "this is a failed deploy rather than an outage. Check the Coolify logs."
      exit 1 ;;
  esac
  sleep 10
done

echo "::error::Still running after 15 minutes. Neither confirming nor denying it"
echo "worked — check Coolify. Giving up quietly would report a green deploy that"
echo "was never seen to finish."
exit 1
