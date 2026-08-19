# Combined manual pass — Plans 1+2. Comments = expected outcome. One billable step, marked.

cp ~/.config/urlbox/config.json ~/.config/urlbox/config.json.bak

# ── logged-out guards ──
urlbox logout
# ok even if already logged out
urlbox whoami
# not logged in — run `urlbox login`, exit 3
urlbox usage
# exit 3
urlbox orgs list
# exit 3
urlbox storage list
# exit 3
urlbox proxies list
# exit 3
urlbox llm list
# exit 3
urlbox auth
# MUST FAIL with unknown command — proves the removed auth command is really gone
urlbox doctor
# table: session/active_org/active_project/render_credential rows FAIL with login hints, summary ✗

# ── login ──
urlbox login
# browser opens, code shown, org/project pickers if several; ends: email + org + project + render credential ready/issued
ls -la ~/.config/urlbox/config.json
# -rw------- (0600)
cat ~/.config/urlbox/config.json
# has session_token, active_org (org_…), active_project (proj_…), api_key AND api_secret
urlbox whoami
# boxed KV: SIGNED IN / ORG / PROJECT
urlbox me
# same (alias)
urlbox usage
# boxed KV: renders used / quota / period
urlbox orgs list
# table, ● on active row
urlbox projects list
# table, ● on active row
urlbox doctor
# all rows ✓ (install_method may warn)

# ── rendering funded by login ──
urlbox screenshot https://example.com --dry-run
# ✓ payload validated, no API call
urlbox screenshot https://example.com --output check.png
# BILLABLE (1 render) — writes ./check.png (output is sandboxed to cwd by design), opens, is example.com
urlbox link https://example.com --format png
# KV incl. full signed URL
curl -sI '<paste the URL>' | head -1
# NOT 401/403 (pair matches)

# ── config keys ──
urlbox config get session_token
# masked (abcd…xy)
urlbox config get session_token --reveal
# full token
urlbox config get active_org
# org_… unmasked
urlbox config get active_project
# proj_… unmasked

# ── projects CRUD ──
urlbox projects create tmp-check
# created, proj_… id shown in JSON; TTY asks "Switch to this project?" — answer n
urlbox projects show tmp-check
# boxed KV: NAME/ID/ENABLED/ENGINE/WEBHOOK KEY (masked)/CREATED
urlbox projects show tmp-check --reveal
# webhook key in full
urlbox projects rename tmp-check tmp-check2
# renamed
urlbox projects disable tmp-check2
# asks "Disable project tmp-check2?" — answer n → stays enabled, says so
urlbox projects disable tmp-check2
# answer y → disabled
urlbox projects enable tmp-check2
# enabled, no gate
urlbox projects defaults set tmp-check2 --json '{"width":1280}'
# set
urlbox projects defaults show tmp-check2
# width 1280
urlbox projects defaults set tmp-check2 --json '{"full_page":true}' --merge
# merged
urlbox projects defaults show tmp-check2
# width 1280 AND full_page true
urlbox projects defaults remove tmp-check2 --yes
# removed
urlbox projects delete tmp-check2
# retype prompt — type it WRONG once → refuses; run again, type correctly → deleted

# ── create --select + delete-active ──
urlbox projects create tmp-sel --select
# Created project tmp-sel (now active)
urlbox whoami
# PROJECT = tmp-sel
urlbox projects delete tmp-sel --yes
# Deleted … (was your active project). Several projects remain, so the TTY shows a picker seeded at the first.
# Pick one → stderr: Now active: <name>, api_secret refreshed. Skip → stderr: Select one with `urlbox projects select`.
urlbox whoami
# picked → PROJECT = <name>; skipped → PROJECT = (none)
urlbox projects select Default
# active again; api_secret refreshed

# ── storage (fake values; writes to prod org, cleaned below) ──
urlbox storage list
# table: BUCKET/ID/PROVIDER/ENDPOINT/KEY/ASSIGNED (or empty)
urlbox storage create --name fake-s3 --provider aws_s3 --bucket fake-bucket --region us-east-1 --key FAKEKEY --secret FAKESECRET
# created store_…; TTY asks assign-to-project — skip
urlbox storage show fake-s3
# boxed KV, KEY/SECRET masked
urlbox storage show fake-s3 --reveal
# secrets in full
urlbox storage update fake-s3 --region eu-west-1
# updated (partial patch)
urlbox projects storage assign Default fake-s3
# Assigned fake-s3 to Default
urlbox projects storage unassign Default
# Unassigned the storage credential from Default
urlbox storage delete fake-s3 --yes
# deleted

# ── proxies (fake values) ──
urlbox proxies list
# table: ID/NAME/URLS(count)/ASSIGNED
urlbox proxies create --name fake-pool --url 'http://user:hunter2@127.0.0.1:9999'
# created pool_…; skip assign
urlbox proxies show fake-pool
# URL shows http://user:****@127.0.0.1:9999 — hunter2 NOT visible
urlbox proxies show fake-pool --reveal
# hunter2 visible
urlbox proxies update fake-pool --url 'http://user:hunter2@127.0.0.1:9998'
# whole URL list replaced (help text warns about this)
urlbox projects proxy assign Default fake-pool
# assigned
urlbox projects proxy unassign Default
# unassigned
urlbox proxies delete fake-pool --yes
# deleted

# ── llm (fake key) ──
urlbox llm list
# table: ID/NAME/PROVIDER/MODEL/ASSIGNED
urlbox llm create --name fake-llm --provider openai --api-key sk-fake123
# created llm_…; skip assign
urlbox llm show fake-llm
# boxed KV, apiKey masked
urlbox llm show fake-llm --reveal
# sk-fake123 visible
urlbox llm update fake-llm --model gpt-5-mini
# updated; provider NOT changeable
urlbox llm test fake-llm
# Connection failed (fake key — expected), exit 1
urlbox llm models fake-llm
# fails (fake key — expected)
urlbox projects llm assign Default fake-llm
# assigned
urlbox projects llm unassign Default
# unassigned
urlbox llm delete fake-llm --yes
# deleted

# ── agent / non-TTY safety ──
echo | urlbox orgs select
# instant clean error naming <name-or-id>, no hang
echo | urlbox projects delete Default
# refuses (needs confirmation / --yes) — does NOT delete, no API call
urlbox whoami --output-format json
# envelope with email/org/project
urlbox whoami --jq .data.email
# bare email
urlbox storage list --output-format json
# raw envelope

# ── logout / re-login ──
urlbox logout
# session revoked; config cleared of session_token/active_org/active_project/api_key/api_secret
urlbox whoami
# exit 3
urlbox login
# second login works end to end

# done — optionally restore: cp ~/.config/urlbox/config.json.bak ~/.config/urlbox/config.json
