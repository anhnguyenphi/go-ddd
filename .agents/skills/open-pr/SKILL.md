---
name: open-pr
description: Use only when the user explicitly asks to ship a finished feature as a pull request. Commits the change with its review brief, re-runs harness-verify --e2e on the committed tree, and only on PASS pushes the branch, opens (or reuses) the pull request, and posts the harness report as proof in a PR comment.
disable-model-invocation: true
---

# Open a PR with live proof

This is the step after `implement-and-verify-loop` once the user says "ship it".
AGENTS.md forbids pushing or opening a pull request unasked. Run this skill
only when the user has asked, and never on your own initiative.

What it does, in order: commit, verify the committed tree, push, open the PR,
and comment the proof. The proof must describe the exact commit the PR holds,
so the harness runs **after** the commit, on a clean tree.

Always pass `GO=/snap/go/current/bin/go` to make.

## 0. Preconditions

- `gh auth status` succeeds. If it doesn't, ask the user to run `! gh auth login`.
- The current branch is not the default branch (`master`). If it is, ask the
  user for a branch name and run `git switch -c <name>`. Never push to `master`.
- `tmp/review-brief.md` exists. If it doesn't, write it first from
  [../implement-and-verify-loop/brief-template.md](../implement-and-verify-loop/brief-template.md),
  using the final report of the work loop.
- Show the user the brief and the list of files you will stage. Wait for their
  OK, and apply their edits to the brief.

## 1. Commit

Skip this step if the change is already committed and the tree is clean.

- Stage the change's files **by path**, memory edits (`CONTEXT.md`,
  `learnings.md`) included. Never use `git add -A` or `git add .`. Never stage
  `vendor/`, `tmp/`, `_harness/checks/`, env files, or secrets.
- `git commit -F tmp/review-brief.md` (`-F` keeps the `##` headings), and end
  the message with the `Co-Authored-By` attribution line.
- Never amend. A fix after this point is a new commit.

## 2. Verify the committed tree

```bash
test -z "$(git status --porcelain)"            # must be clean, or the report says (dirty)
make GO=/snap/go/current/bin/go harness-verify ARGS=--e2e; echo "exit=$?"
```

The report is `tmp/harness/<sandbox>/report.md`, where the sandbox is named after the
worktree directory, or `$SANDBOX`. Check every line below before going on:

- Exit code `0`, and the report contains `Result: PASS`.
- The `Git:` line is `<branch>@<git rev-parse --short HEAD>`, with no `(dirty)`.
- The report has no `STALE` line.
- The E2E section says `PASS`, not `Not run`.

On exit `1` (FAIL) or `2` (HARNESS ERROR), or on any mismatch, **stop**. Keep
the commit local. Don't push and don't open the PR. Report the result and the
failing report sections to the user. A FAIL means going back to the work loop;
a HARNESS ERROR means fixing the environment, not the code.

## 3. Push

`git push -u origin <branch>`. Never use `--force`, and never push the default branch.

## 4. Open the PR (or reuse it)

```bash
gh pr view --json number,url,state 2>/dev/null   # does the branch already have an open PR?
```

- An open PR exists: reuse it. Don't change its title or body.
- None: the title is the first line of the brief. The body is the brief without
  that line, plus the attribution line at the end:

```bash
tail -n +3 tmp/review-brief.md > tmp/pr-body.md
printf '\n🤖 Generated with [Claude Code](https://claude.com/claude-code)\n' >> tmp/pr-body.md
gh pr create --base master --head <branch> --title "<brief subject>" --body-file tmp/pr-body.md
```

## 5. Post the proof comment

Build `tmp/pr-proof.md` from the template below, using the report from step 2.
Paste it whole, except its last line (`Logs: ... · Responses: ... · Failure
evidence: ...`): those are local paths that a reader can't open.

```bash
report="tmp/harness/<sandbox>/report.md"
{
  cat <<EOF
## Harness verify proof

Commit: \`$(git rev-parse HEAD)\`
Command: \`make harness-verify ARGS=--e2e\` -> exit 0 (PASS)
Ran on the committed tree (clean), with the real api and worker, Postgres, Kafka, and WireMock stubs.

<details><summary>Sandbox report</summary>

EOF
  sed '$d' "$report"
  cat <<'EOF'

</details>

Local run by the coding agent, not independently checked. The CI `e2e` job reruns the harness on the pushed commit.
EOF
} > tmp/pr-proof.md
gh pr comment <number> --body-file tmp/pr-proof.md
```

To ship new commits on the same branch, run the skill again. Each run adds a
new comment for the new HEAD and leaves old comments alone.

## Stop and ask when

- You can't tell which uncommitted files belong to the change.
- The remote branch has diverged (the push is rejected). Don't force-push or
  rebase without the user's say-so.
- The PR is closed or merged. Ask whether to open a new one.

## Report to the user

```
PR: <url> (created | reused)
COMMIT: <sha> <subject>
PROOF: <comment url>, Result: PASS, Git: <branch>@<sha>, e2e PASS, <n> checks, 0 unmatched, 0 unexpected errors
```

Or, when the skill stopped:

```
STOPPED at <step>: <reason>
COMMIT: <sha> (local, not pushed) | none
EVIDENCE: <report Result line and failing sections>
```
