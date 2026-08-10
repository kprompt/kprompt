#!/usr/bin/env bash
# beforeShellExecution: enforce the kprompt branch naming convention.
# Fires on git branch-creating commands (checkout -b / switch -c / branch <name>).
# Returns permission "ask" when the proposed name doesn't match the convention.
# Fail open: any parse error or missing python3 -> allow.
set -uo pipefail

input=$(cat || true)
[[ -z "${input}" ]] && { echo '{ "permission": "allow" }'; exit 0; }

if ! command -v python3 >/dev/null 2>&1; then
  echo '{ "permission": "allow" }'
  exit 0
fi

HOOK_INPUT="${input}" python3 <<'PY'
import os, json, re, shlex

try:
    data = json.loads(os.environ.get("HOOK_INPUT", "") or "{}")
except Exception:
    print('{ "permission": "allow" }'); raise SystemExit(0)

command = (data.get("command") or "")
if not command.strip():
    print('{ "permission": "allow" }'); raise SystemExit(0)

NAME_RE = re.compile(
    r'^(feat|fix|docs|test|chore|refactor|perf|build|ci|sec|hotfix)/[a-z0-9][a-z0-9._-]*$'
)

GIT_GLOBAL_WITH_VALUE = {"-C", "-c", "--git-dir", "--work-tree", "--namespace", "--exec-path"}
BRANCH_NON_CREATE = {
    "-d", "-D", "--delete", "-a", "--all", "-r", "--remotes",
    "--list", "-l", "--merged", "--no-merged", "--contains",
    "-v", "-vv", "--show-current", "--edit-description",
}

def candidate_from_segment(seg):
    try:
        toks = shlex.split(seg)
    except Exception:
        return None
    if "git" not in toks:
        return None
    args = toks[toks.index("git") + 1:]
    j = 0
    while j < len(args):
        a = args[j]
        if a in GIT_GLOBAL_WITH_VALUE:
            j += 2; continue
        if a.startswith("-"):
            j += 1; continue
        break
    if j >= len(args):
        return None
    sub = args[j]
    subargs = args[j + 1:]

    if sub == "checkout":
        for k, t in enumerate(subargs):
            if t in ("-b", "-B") and k + 1 < len(subargs):
                return subargs[k + 1]
        return None
    if sub == "switch":
        for k, t in enumerate(subargs):
            if t in ("-c", "-C") and k + 1 < len(subargs):
                return subargs[k + 1]
        return None
    if sub == "branch":
        flags = {t for t in subargs if t.startswith("-")}
        if flags & BRANCH_NON_CREATE:
            return None
        positionals = [t for t in subargs if not t.startswith("-")]
        return positionals[0] if positionals else None
    return None

segments = re.split(r'&&|\|\||;|\n', command)
bad = None
for seg in segments:
    name = candidate_from_segment(seg)
    if name and not NAME_RE.match(name):
        bad = name
        break

if bad is None:
    print('{ "permission": "allow" }')
    raise SystemExit(0)

user_message = (
    f"Branch name '{bad}' doesn't match the kprompt convention "
    "(<type>/<kebab-summary>[-issue], e.g. feat/daemonset-wait-106). "
    "See .cursor/rules/branch-naming.mdc."
)
agent_message = (
    f"Proposed branch '{bad}' is invalid. Use "
    "^(feat|fix|docs|test|chore|refactor|perf|build|ci|sec|hotfix)/[a-z0-9][a-z0-9._-]*$ "
    "— lowercase kebab-case with a type prefix, e.g. feat/daemonset-wait-106 or "
    "fix/plan-deny-spacing. Rename before creating the branch."
)
print(json.dumps({
    "permission": "ask",
    "user_message": user_message,
    "agent_message": agent_message,
}))
PY
exit 0
