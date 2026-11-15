# Review brief template

The review brief is the body of the commit that holds a finished change.
GitHub copies the body of a single-commit branch into the pull request
description, so a reviewer who did not see the agent session can read it
there. It is a reading aid, not a gate: it never changes `RESULT`.

Fill the template from the final report fields. Do not write new claims here:
`Problem` is `PROBLEM`, the criteria are `CRITERIA`, `Solution` is `CHANGE`,
and the local line under `Proof` is `EVIDENCE`.

````
<subject: what the change does, imperative, at most 72 characters>

## Problem

<PROBLEM: 2 to 4 sentences. What was wrong or missing, and for whom.>

## Acceptance criteria

<n> criteria: <a> committed test, <b> seen live only, <c> NOT VERIFIED

1. <observable effect, as written at step 1>
   **Proof:** committed test, `<path/to/file_test.go>` `<TestName or scenario name>`
2. <observable effect>
   **Proof:** seen live only, not in this PR: <what was asserted, in one line>
3. <observable effect>
   **Proof:** NOT VERIFIED

## Solution

<CHANGE: one paragraph. What was changed and where.>

## Proof

- Local, on the working tree before this commit was created:
  `make harness-verify ARGS=--e2e` -> PASS (e2e ok, 0 unmatched, 0 unexpected errors);
  `scripts/check.sh` -> OK
- Independent: the CI "e2e" job on the pull request reruns the harness on
  the pushed commit. CI also reruns every committed test cited above.

Written by the coding agent that made the change. Not independently checked.
````

## Proof grades

Each criterion carries exactly one grade.

- **committed test**: a Go test or scenario in this change, cited by file
  path and name so a reader can open it. For a test the harness report does
  not show (unit, contract, integration), add the command that ran it and its
  result, for example `(make test -> ok)`.
- **seen live only, not in this PR**: a scratch check showed the effect.
  Scratch checks are gitignored and are not evidence of coverage, so write out
  the assertion in one line and do not cite the check by name. For a behavior
  change this grade means definition of done item 4 is not met: promote the
  check to a Go test first if you can.
- **NOT VERIFIED**: nothing showed the effect. Say so. Do not leave the
  criterion out.

## Rules

- Criteria are the ones written at step 1, in the same order and words. If one
  changed during the work, add a line under it: `Changed: <from> -> <to>, <why>`.
- If the task request gave no explicit criteria, add `(written by the agent)`
  to the count line.
- Restate the request. Do not quote the session, and do not write "as
  discussed" or "the user asked": the text stays in git history.
- The body is Markdown, because GitHub renders the pull request description:
  `##` headings, numbered criteria, `- ` bullets, backticks for paths, test
  names and commands, `**bold**` for the proof label. Keep it readable as
  plain text in `git log`: no tables, no HTML, no images, and no `- [ ]`
  checkboxes (GitHub turns them into a task counter).
- Git treats a line that starts with `#` as a comment when a message goes
  through an editor. `git commit -F` keeps the headings. To edit the message
  later, use `git commit --amend --cleanup=whitespace`, or the headings are
  deleted.
- Nothing a reader cannot reach: no sandbox name, no scratch check name, no
  `tmp/` path, no local SHA with `(dirty)`.
- Keep it under about 40 lines.
