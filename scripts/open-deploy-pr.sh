#!/usr/bin/env bash
# Open a repin PR on homelab-gitops bumping iron-temple to the given image refs.
# API-only (no git checkout of gitops); auth via GITOPS_TOKEN (write:repository on
# homelab-gitops, non-admin iron-temple-deployer bot).
#   $1 = version (e.g. 0.1.0)   $2 = api image ref   $3 = ui image ref
#
# Only ever one deploy PR open at a time: a repin overwrites both image lines
# wholesale rather than patching them, so the newest release is the only one worth
# merging — its images already contain every commit the pending older ones carry.
# Leaving two open is what breaks them. Both get cut from the same main and touch
# the same two lines, so whichever merges second is left conflicting (this stranded
# v0.29.0 in gitops#437 behind v0.28.1). Superseding the older PR here keeps the
# queue at depth one, and re-cutting our own branch from current main on every run
# makes a repeat release of one version idempotent instead of a 409 against the
# branch it left behind.
set -euo pipefail
VERSION="$1"; API_REF="$2"; UI_REF="$3"
: "${GITOPS_TOKEN:?GITOPS_TOKEN required}"

API="http://gitea-http.gitea.svc.cluster.local:3000/api/v1/repos/gitadmin/homelab-gitops"
FILE="services/iron-temple/deployment.yaml"
BRANCH="deploy/iron-temple-v${VERSION}"
PAGE_LIMIT=50 # Gitea caps /pulls page size here anyway

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

# Existence probe that doesn't go through req(), whose 404 diagnostics would read
# as a failure when the answer we want is just "no".
branch_exists() {
  [ "$(curl -sS -o /dev/null -w '%{http_code}' \
        -H "Authorization: token ${GITOPS_TOKEN}" "${API}/branches/$1")" = "200" ]
}

# 0. supersede whatever is still pending. A stale branch can't be salvaged by
# rewriting its content: git merges by ancestry, so a branch cut from an older main
# still conflicts on the image lines even once its file matches byte for byte. It
# has to be re-cut from current main, which means retiring the PR pointing at it.
# Best-effort throughout — the images and tag are already pushed by this point, so
# a hiccup here should cost us a conflict to clean up by hand, not the whole PR.
# Paged through rather than read off page one: gitops carries a long renovate tail,
# and a deploy PR pushed past the first page would silently go un-superseded — the
# exact two-open-PRs state this guards against, minus the evidence.
superseded=()
still_open=()
open_prs="[]"
page=1
while :; do
  batch="$(req GET "${API}/pulls?state=open&limit=${PAGE_LIMIT}&page=${page}" || echo '[]')"
  batch="$(printf '%s' "$batch" | jq -c 'if type == "array" then . else [] end' 2>/dev/null || echo '[]')"
  open_prs="$(jq -nc --argjson a "$open_prs" --argjson b "$batch" '$a + $b')"
  [ "$(printf '%s' "$batch" | jq 'length')" -lt "${PAGE_LIMIT}" ] && break
  page=$((page + 1))
  if [ "$page" -gt 20 ]; then
    echo "warning: stopped scanning open PRs at page 20 — a pending deploy PR past" \
         "that point will not be superseded." >&2
    break
  fi
done
while read -r num ref ver; do
  [ -n "${num}" ] || continue
  # A forced workflow_dispatch can re-release an old version out of order; never
  # let that retire a newer deploy that legitimately supersedes *us*.
  if [ "$(printf '%s\n%s\n' "${ver}" "${VERSION}" | sort -V | head -1)" != "${ver}" ]; then
    echo "warning: gitops#${num} deploys v${ver}, newer than v${VERSION} — leaving it open." >&2
    continue
  fi
  echo "Superseding gitops#${num} (v${ver})"
  jq -n '{state:"closed"}' > /tmp/close.json
  # Only claim the supersede once the close actually took. Recording it either way
  # would put "Supersedes vX" on a PR that still has vX open next to it, telling the
  # operator the queue is at depth one at the moment it isn't.
  if req PATCH "${API}/pulls/${num}" /tmp/close.json >/dev/null; then
    req DELETE "${API}/branches/${ref}" >/dev/null \
      || echo "warning: could not delete ${ref} (continuing)" >&2
    superseded+=("v${ver} (gitops#${num})")
  else
    echo "warning: could not close gitops#${num} — it stays open and will conflict." >&2
    still_open+=("v${ver} (gitops#${num})")
  fi
done < <(printf '%s' "$open_prs" | jq -r '
  .[] | select(.base.ref == "main")
      | select(.head.ref | startswith("deploy/iron-temple-v"))
      | [.number, .head.ref, (.head.ref | ltrimstr("deploy/iron-temple-v"))] | @tsv')

# Our own branch also outlives a half-finished earlier run — one that pushed the
# commit but never opened the PR, or whose PR was closed by hand — and the loop
# above only sees branches that still have one. Drop it so step 3 can re-cut it.
if branch_exists "${BRANCH}"; then
  echo "Re-cutting ${BRANCH} from current main"
  req DELETE "${API}/branches/${BRANCH}" >/dev/null
fi

# 1. current file (base64 content + blob sha)
resp="$(req GET "${API}/contents/${FILE}?ref=main")"
sha="$(printf '%s' "$resp" | jq -r '.sha')"
printf '%s' "$resp" | jq -r '.content' | tr -d '\n' | base64 -d > /tmp/dep.yaml

# 2. repin the two image lines (preserve indentation)
sed -i -E "s#( *image: ).*iron-temple-api:.*#\1${API_REF}#" /tmp/dep.yaml
sed -i -E "s#( *image: ).*iron-temple-ui:.*#\1${UI_REF}#"   /tmp/dep.yaml
newcontent="$(base64 -w0 < /tmp/dep.yaml)"

# 3. commit onto a fresh branch created from main. new_branch is what makes the PR
# mergeable: it parents the commit on main *as of now*, not on whatever main looked
# like when this release started building.
jq -n --arg m "deploy(iron-temple): v${VERSION}" --arg c "$newcontent" \
      --arg s "$sha" --arg nb "$BRANCH" \
   '{message:$m, content:$c, sha:$s, branch:"main", new_branch:$nb}' > /tmp/put.json
req PUT "${API}/contents/${FILE}" /tmp/put.json >/dev/null

# 4. open the PR — body = changelog (from the release job) + the pinned image refs.
NOTES_FILE="${NOTES_FILE:-/tmp/notes.md}"
if [ -s "${NOTES_FILE}" ]; then
  notes="$(cat "${NOTES_FILE}")"
else
  notes="- (no notable changes)"
fi
{
  echo "Automated repin from the iron-temple release workflow."
  echo
  echo "## Changes"
  echo "${notes}"
  echo
  echo "- api: ${API_REF}"
  echo "- ui: ${UI_REF}"
  # Recorded here rather than as a comment on the closed PR: GITOPS_TOKEN carries
  # write:repository, which doesn't cover issue comments.
  if [ ${#superseded[@]} -gt 0 ]; then
    sup="$(printf '%s, ' "${superseded[@]}")"
    echo
    echo "Supersedes ${sup%, } — this release's images already include those commits."
  fi
  # On the PR rather than only in the release job's stderr, which nobody reads once
  # the run is green: a supersede that didn't take is exactly the two-open-PRs state
  # that needs a human, so it has to be visible where the human is looking.
  if [ ${#still_open[@]} -gt 0 ]; then
    sto="$(printf '%s, ' "${still_open[@]}")"
    echo
    echo "⚠️ Could not close ${sto%, } — still open and will conflict with this PR."
    echo "Close those by hand before merging this one."
  fi
} > /tmp/pr-body.md

jq -n --arg h "$BRANCH" --arg t "deploy(iron-temple): v${VERSION}" --rawfile b /tmp/pr-body.md \
   '{head:$h, base:"main", title:$t, body:$b}' > /tmp/pr.json
req POST "${API}/pulls" /tmp/pr.json >/dev/null
echo "Opened deploy PR: ${BRANCH}"
