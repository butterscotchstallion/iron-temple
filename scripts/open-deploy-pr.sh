#!/usr/bin/env bash
# Open a repin PR on homelab-gitops bumping iron-temple to the given image refs.
# API-only (no git checkout of gitops); auth via GITOPS_TOKEN (write:repository on
# homelab-gitops, non-admin iron-temple-deployer bot).
#   $1 = version (e.g. 0.1.0)   $2 = api image ref   $3 = ui image ref
set -euo pipefail
VERSION="$1"; API_REF="$2"; UI_REF="$3"
: "${GITOPS_TOKEN:?GITOPS_TOKEN required}"

API="http://gitea-http.gitea.svc.cluster.local:3000/api/v1/repos/gitadmin/homelab-gitops"
FILE="services/iron-temple/deployment.yaml"
BRANCH="deploy/iron-temple-v${VERSION}"

# req METHOD URL [json-datafile] — prints the response body; on HTTP >= 400 prints
# the method/path/status AND the API's error message (so a 403 says *why*), then fails.
req() {
  local method="$1" url="$2" data="${3:-}"
  local curlargs=(-sS -X "$method" -H "Authorization: token ${GITOPS_TOKEN}"
                  -H "Content-Type: application/json" -w $'\n%{http_code}')
  [ -n "$data" ] && curlargs+=(--data "@${data}")
  local out code body
  out="$(curl "${curlargs[@]}" "$url")"
  code="${out##*$'\n'}"
  body="${out%$'\n'*}"
  if [ "${code}" -ge 400 ]; then
    echo "ERROR: ${method} ${url#*homelab-gitops} -> HTTP ${code}" >&2
    echo "  ${body}" | head -c 600 >&2; echo >&2
    if [ "${code}" = "403" ]; then
      echo "  hint: iron-temple-deployer must be a WRITE collaborator on" \
           "homelab-gitops, and GITOPS_TOKEN scoped write:repository." >&2
    fi
    return 1
  fi
  printf '%s' "${body}"
}

# 1. current file (base64 content + blob sha)
resp="$(req GET "${API}/contents/${FILE}?ref=main")"
sha="$(printf '%s' "$resp" | jq -r '.sha')"
printf '%s' "$resp" | jq -r '.content' | tr -d '\n' | base64 -d > /tmp/dep.yaml

# 2. repin the two image lines (preserve indentation)
sed -i -E "s#( *image: ).*iron-temple-api:.*#\1${API_REF}#" /tmp/dep.yaml
sed -i -E "s#( *image: ).*iron-temple-ui:.*#\1${UI_REF}#"   /tmp/dep.yaml
newcontent="$(base64 -w0 < /tmp/dep.yaml)"

# 3. commit onto a fresh branch created from main
jq -n --arg m "deploy(iron-temple): v${VERSION}" --arg c "$newcontent" \
      --arg s "$sha" --arg nb "$BRANCH" \
   '{message:$m, content:$c, sha:$s, branch:"main", new_branch:$nb}' > /tmp/put.json
req PUT "${API}/contents/${FILE}" /tmp/put.json >/dev/null

# 4. open the PR
jq -n --arg h "$BRANCH" --arg t "deploy(iron-temple): v${VERSION}" \
      --arg b "Automated repin from the iron-temple release workflow.

- api: ${API_REF}
- ui: ${UI_REF}" \
   '{head:$h, base:"main", title:$t, body:$b}' > /tmp/pr.json
req POST "${API}/pulls" /tmp/pr.json >/dev/null
echo "Opened deploy PR: ${BRANCH}"
