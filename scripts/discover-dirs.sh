#!/usr/bin/env sh
# Print this repo's Python and JS/TS project directories as two env-style lines:
#
#   PY_DIRS=<space-separated, possibly empty>
#   JS_DIRS=<space-separated, possibly empty>
#
# Two lines, nothing else on stdout. ci-lint.yml redirects them straight into
# $GITHUB_ENV and every later step word-splits the value:
#
#   [ -n "${PY_DIRS}" ] || { echo "no Python project — skipping"; exit 0; }
#   for d in ${PY_DIRS}; do (cd "${d}" && ruff check .); done
#
# So the contract is exact, and every way of missing it is silent and GREEN: a
# dropped key leaves the variable unset and lints nothing, and a value of " "
# instead of "" is non-empty to `[ -n ]`, so it skips the "no project" branch
# and then loops zero times. Emit empty, not blank.
#
# WHY A SCRIPT AND NOT AN INLINE STEP: this logic used to live inline in
# ci-lint.yml. A shell harness cannot test logic embedded in workflow YAML
# without re-implementing it, and a gate that tests its own copy of the code
# stays green while the workflow drifts away from it.
#
# NOTHING here hardcodes a project directory. Repos in this fleet put Python in
# radar/ (tempestwx) or renderer/ (dras), and JS at the repo root (ws4kp,
# tempest-display) or in web/ (tempestwx). A `working-directory:` per step would
# make the calling workflow repo-specific, which defeats sharing it unchanged.
#
# The prune list is shared knowledge with chart-list.sh and dockerfile-list.sh —
# "directories whose contents belong to someone else" — so keep the three in
# step. `.ci-shared` is a legacy name from a CI layout this pipeline no longer
# uses; it stays because pruning a directory that never appears costs nothing,
# while discovering a dependency's project as this repo's own is what the list
# exists to prevent.
#
# `testdata` IS IN THE LIST FOR A DIFFERENT REASON FROM THE REST, and it is not
# a vendored tree. The Go toolchain defines a directory of that name as excluded
# from the build, so its contents are FIXTURES — a repo's own, but not its
# shipped source. Left unpruned, a Go repo with testdata/**/package.json was
# reported as a Node project, and the setup action then ran actions/setup-node
# against a .node-version that copier's `_exclude` never wrote, killing the job
# outright. The same file as a Chart.yaml gets a fixture chart linted and
# dependency-resolved; as a Dockerfile it gets hadolint findings nobody can act
# on. All four callers of this list prune it now.
#
# Chart discovery is NOT here — that is scripts/chart-list.sh's single job.
#
# POSIX sh — runs identically on a runner and on macOS.
set -eu

prune='/(\.git|\.ci-shared|node_modules|vendor|\.venv|\.worktrees|testdata)/'

# Reduce a newline-separated list of marker files to the sorted, de-duplicated,
# space-separated list of the directories holding them.
#
# NOT `xargs -n1 dirname`, which the inline original used: xargs splits its input
# on ANY whitespace, so a marker at "./my web/package.json" becomes two arguments
# and the result is "." plus "web" — two directories that were never found, one
# of which is the repo root. Verified on macOS. Splitting on newline only, the
# way chart-list.sh does, is what keeps a real path intact.
dirs_of() {
  oldifs=$IFS
  IFS='
'
  # shellcheck disable=SC2086  # deliberate newline-only split, per IFS above
  set -- $1
  IFS=$oldifs

  out=''
  for f in "$@"; do
    d=$(dirname "${f}")
    # A space-separated list cannot carry a path containing a space. REFUSE
    # rather than emit a mangled one: the caller would `cd` into directories
    # that do not exist, or worse, into the repo root and lint the whole tree.
    case "${d}" in
      *[[:space:]]*)
        echo "discover-dirs: ${d}: project directory contains whitespace" >&2
        echo "discover-dirs: PY_DIRS/JS_DIRS are space-separated and cannot carry it" >&2
        exit 1
        ;;
    esac
    out="${out}${d}
"
  done

  printf '%s' "${out}" | sort -u | tr '\n' ' '
}

# `|| true` because grep exits 1 when it filters everything out, which is the
# ordinary "this repo has no Python" case, not an error.
py_files=$(find . \( -name pyproject.toml -o -name requirements.txt \) \
             | grep -Ev "${prune}" || true)
js_files=$(find . -name package.json \
             | grep -Ev "${prune}" || true)

# Assign FIRST, print second. `echo "PY_DIRS=$(dirs_of …)"` looks equivalent and
# is not: dirs_of's `exit 1` only leaves the command substitution's subshell, and
# the surrounding `echo` then succeeds — so the refusal above wrote its message to
# stderr, printed "PY_DIRS=" to stdout anyway, and exited 0. A bare assignment
# takes the substitution's exit status, so `set -e` aborts before anything is
# written. Caught by tests/discovery-test.sh; do not re-inline these.
py=$(dirs_of "${py_files}")
js=$(dirs_of "${js_files}")

echo "PY_DIRS=${py}"
echo "JS_DIRS=${js}"
