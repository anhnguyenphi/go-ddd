# Review brief template

The review brief is the body of the commit that holds a finished change, and
the description of its pull request (the `open-pr` skill passes it to
`gh pr create`). It is for a reviewer who did not see the agent session. It
is a reading aid, not a gate: it never changes `RESULT`.

The brief answers two questions: **what must be true** (the acceptance
criteria) and **how the change makes it true** (the solution). It does not
carry the harness run. `open-pr` posts the sandbox report for the pushed
commit as a PR comment, and CI reruns it. Don't paste the run results,
exit codes, or a `## Proof` section into the brief.

Fill the template from the final report fields. Do not write new claims here:
`Problem` is `PROBLEM`, the criteria are `CRITERIA`, and `Solution` expands
`CHANGE`.

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

<CHANGE, expanded: 2 to 5 sentences on the approach. Describe the path a request or
event now takes, in the context's terms, and which criterion each part serves.>

Key decisions:

- <decision>: <why this way, and the alternative you didn't take>

Where to look:

- `<path>`: <what changed there, in one line>

Not in this change: <what a reviewer might expect that isn't here, and why> | nothing
````

## Proof grades

Each criterion carries exactly one grade. The grade says what pins the
criterion. It is not a log of what ran.

- **committed test**: a Go test or scenario in this change, cited by file
  path and name so a reader can open it.
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
- The solution explains reasons. A reader can get the "what" from the diff,
  so spend the words on the "why": each key decision names its reason.
  "Where to look" lists only the files that carry the logic, in reading
  order, and leaves out mechanical edits (generated code, mocks, wiring).
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
- Keep it under about 50 lines.
