#!/usr/bin/env sh
# Print each of THIS repo's chart directories, one per line, relative to $PWD.
#
# THE SINGLE SOURCE OF TRUTH for "which Chart.yaml files are ours". This glob
# previously existed in two places — ci-lint.yml and chart-deps.sh — and produced
# two separate fail-silent bugs, so it lives here and is tested.
#
# The exclusion must skip SUBCHARTS without skipping the chart itself:
#
#   ./charts/Chart.yaml                    KEEP  — the fleet-standard location
#   ./charts/charts/dep/Chart.yaml         DROP  — an extracted dependency
#
# The stock template used `-not -path '*/charts/*'`, which ALSO excludes
# ./charts/Chart.yaml, so every chart task silently no-opped and still exited 0.
# Anchoring to '*/charts/*/Chart.yaml' keeps the chart and drops its subcharts,
# because a subchart always sits one directory deeper.
#
# THE VENDORED TREES ARE PRUNED TOO, and the list is the one discover-dirs.sh
# already carries. Pruning only `.git` returned SIX directories from
# tests/fixtures/pruned where one is the repo's own, so a dependency's chart in
# node_modules/ or vendor/ was linted, templated and dependency-resolved as if
# the consumer owned it.
#
# THE LIST IS SHARED KNOWLEDGE with discover-dirs.sh and dockerfile-list.sh —
# "directories whose contents belong to someone else" — so keep the three in
# step. `.ci-shared` is a legacy name from a CI layout this pipeline no longer
# uses; it stays because pruning a directory that never appears costs nothing,
# while resolving a foreign chart as this repo's own is the exact damage the list
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
# POSIX sh — runs identically on a runner and on macOS.
set -eu

charts=$(find . -name Chart.yaml \
  -not -path '*/.git/*' \
  -not -path '*/.ci-shared/*' \
  -not -path '*/node_modules/*' \
  -not -path '*/vendor/*' \
  -not -path '*/.venv/*' \
  -not -path '*/.worktrees/*' \
  -not -path '*/testdata/*' \
  -not -path '*/charts/*/Chart.yaml')
[ -n "${charts}" ] || exit 0

# Split on NEWLINE ONLY so a path containing spaces survives, and stay in the
# current shell so a caller's `set -e` still aborts on failure — piping into
# `while read` would swallow it (SC2044).
oldifs=$IFS
IFS='
'
# shellcheck disable=SC2086  # deliberate newline-only split, per IFS above
set -- ${charts}
IFS=$oldifs

for c in "$@"; do
  dirname "${c}"
done
