#!/usr/bin/env sh
# Emails a CI-failure card via the mail-relay. Replaces the ~13-line inline SMTP
# block that used to be duplicated in every workflow's failure step.
#
# Usage (in a workflow job):
#   - name: Notify on failure
#     if: failure()
#     env:
#       WORKFLOW: ${{ github.workflow }}
#       REPO: ${{ github.repository }}
#       BRANCH: ${{ github.ref_name }}
#       RUN_URL: ${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}
#       SHA: ${{ github.sha }}
#     run: ./scripts/ci-notify.sh
#
# The relay (services/mail-relay) owns delivery — retry + on-disk spool — so a
# mail-server blip queues instead of dropping. See docs/mail-relay-plan.md.
set -eu

RELAY="${MAIL_RELAY:-http://mail-relay.mail.svc.cluster.local/send}"
WORKFLOW="${WORKFLOW:-workflow}"
REPO="${REPO:-?}"
BRANCH="${BRANCH:-?}"
RUN_URL="${RUN_URL:-#}"
short_sha=$(printf '%s' "${SHA:-}" | cut -c1-7)

html=$(printf \
  '<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><style>body{margin:0;padding:24px 16px;background:#f1f5f9;font-family:-apple-system,BlinkMacSystemFont,Segoe UI,Roboto,sans-serif;}.card{max-width:600px;margin:0 auto;background:#fff;border-radius:10px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,.08);}.hdr{padding:26px 30px;background:#dc2626;color:#fff;}.badge{display:inline-block;font-size:13px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;background:rgba(255,255,255,.2);border-radius:5px;padding:3px 11px;margin-bottom:12px;}.title{font-size:26px;font-weight:700;line-height:1.2;margin-bottom:6px;}.sub{font-size:15px;opacity:.85;}.body{padding:26px 30px;}.sec{font-size:12px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:#94a3b8;margin-bottom:10px;padding-bottom:5px;border-bottom:1px solid #f1f5f9;}.tbl{width:100%%;border-collapse:collapse;font-size:14px;margin-bottom:24px;}.tbl td{padding:7px 10px;vertical-align:top;border-bottom:1px solid #f8fafc;}.tbl td.k{color:#64748b;white-space:nowrap;}.tbl td.v{color:#1e293b;font-family:Courier New,monospace;word-break:break-all;}.tbl tr:last-child td{border-bottom:none;}.btn{display:inline-block;padding:12px 26px;background:#3b82f6;color:#fff;text-decoration:none;border-radius:6px;font-size:15px;font-weight:600;}.footer{padding:14px 30px;background:#f8fafc;border-top:1px solid #e2e8f0;font-size:13px;color:#94a3b8;text-align:center;}</style></head><body><div class="card"><div class="hdr"><div class="badge">❌ failed</div><div class="title">%s</div><div class="sub">📦 %s &nbsp;·&nbsp; 🌿 %s</div></div><div class="body"><div class="sec">🏷️ Details</div><table class="tbl"><tr><td class="k">repository</td><td class="v">%s</td></tr><tr><td class="k">branch</td><td class="v">%s</td></tr><tr><td class="k">commit</td><td class="v">%s</td></tr></table><a href="%s" class="btn">🔗 View Run</a></div><div class="footer">🔔 Sent by Gitea CI</div></div></body></html>' \
  "$WORKFLOW" "$REPO" "$BRANCH" "$REPO" "$BRANCH" "$short_sha" "$RUN_URL")

jq -nc --arg s "CI FAILED: ${WORKFLOW}" --arg h "$html" \
  '{from:"ci", name:"CI", subject:$s, html:$h}' \
  | curl -sf -X POST "$RELAY" \
      -H 'Content-Type: application/json' --data-binary @- \
      --connect-timeout 10 --retry 6 --retry-delay 10 --retry-connrefused --retry-all-errors \
      >/dev/null || echo "ci-notify: relay POST failed (curl exit $?)"
