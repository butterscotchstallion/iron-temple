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
auth=(-H "Authorization: token ${GITOPS_TOKEN}")

# 1. fetch the current file (Gitea returns base64 content + its blob sha)
resp="$(curl -sSf "${auth[@]}" "${API}/contents/${FILE}?ref=main")"
sha="$(printf '%s' "$resp" | jq -r '.sha')"
printf '%s' "$resp" | jq -r '.content' | tr -d '\n' | base64 -d > /tmp/dep.yaml

# 2. repin the two image lines (preserve indentation)
sed -i -E "s#( *image: ).*iron-temple-api:.*#\1${API_REF}#" /tmp/dep.yaml
sed -i -E "s#( *image: ).*iron-temple-ui:.*#\1${UI_REF}#"   /tmp/dep.yaml
newcontent="$(base64 -w0 < /tmp/dep.yaml)"

# 3. commit onto a fresh branch created from main (Gitea makes new_branch from branch)
jq -n --arg m "deploy(iron-temple): v${VERSION}" --arg c "$newcontent" \
      --arg s "$sha" --arg nb "$BRANCH" \
   '{message:$m, content:$c, sha:$s, branch:"main", new_branch:$nb}' \
 | curl -sSf "${auth[@]}" -H "Content-Type: application/json" -X PUT \
     --data @- "${API}/contents/${FILE}" >/dev/null

# 4. open the PR
jq -n --arg h "$BRANCH" --arg t "deploy(iron-temple): v${VERSION}" \
      --arg b "Automated repin from the iron-temple release workflow.

- api: ${API_REF}
- ui: ${UI_REF}" \
   '{head:$h, base:"main", title:$t, body:$b}' \
 | curl -sSf "${auth[@]}" -H "Content-Type: application/json" -X POST \
     --data @- "${API}/pulls" >/dev/null
echo "Opened deploy PR: ${BRANCH}"
