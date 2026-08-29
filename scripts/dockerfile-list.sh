#!/usr/bin/env sh
# Print each of THIS repo's Dockerfiles and Containerfiles, one per line,
# relative to $PWD.
#
# THE SINGLE SOURCE OF TRUTH for "which Dockerfiles are ours", the same job
# chart-list.sh does for charts. This glob previously existed in THREE places —
# .github/actions/setup/detect.sh, ci-lint.yml's hadolint step, and a copy of
# the extension filter inside each — and the copies disagreed:
#
#   detect.sh          pruned every dot-directory (`-name '.*'`)
#   ci-lint.yml's find pruned none
#
# So a repo with `.devcontainer/Dockerfile`, a very common layout, got
# docker=false from detect.sh — the setup action therefore never installed
# hadolint — while ci-lint.yml's own find handed that same file to
# `xargs hadolint`. The step died with `hadolint: command not found`, exit 127,
# propagated by `set -euo pipefail`, blaming a Dockerfile that was fine. Both
# callers now read this one answer, so they cannot disagree again.
#
# DOT-DIRECTORIES ARE NOT PRUNED, deliberately. Of the two ways to make the
# callers agree, this is the loud one: a .devcontainer/ or .docker/ Dockerfile
# is found, hadolint is installed, and it gets linted. Pruning them instead
# would have made the disagreement disappear by silently linting nothing, which
# is the failure class this repo exists to remove. The vendored trees below are
# named explicitly instead — the same list discover-dirs.sh and chart-list.sh
# carry, `.ci-shared` included: it is a legacy name from a CI layout this
# pipeline no longer uses, kept because pruning a directory that never appears
# costs nothing.
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
# THE EXTENSION FILTER IS ANCHORED TO /<name>. and must stay that way. A blanket
# `\.(json|md|txt)$` also strips every package.json, which is how Node became
# undetectable the first time this was written. Dockerfile.<variant> is a real
# Dockerfile and is kept; Dockerfile.json is tooling junk and would fail
# hadolint as a parse error, blaming the repo for a file nobody wrote.
#
# Containerfile is the OCI/Podman spelling and hadolint parses it identically.
# Omitting it was FAIL-SILENT: docker=false gave variant=library, and build,
# smoke and release-image vanished from that repo's pipeline with no error.
#
# NO `cd` HERE, deliberately — the same trap chart-deps.sh and chart-drift.sh
# document. It operates on $PWD, which is $GITHUB_WORKSPACE in CI.
#
# POSIX sh — runs identically on a runner and on macOS.
set -eu

# `|| true` because grep exits 1 when it filters everything out, which is the
# ordinary "this repo's only Dockerfile is a Dockerfile.json" case, not an error.
files=$(find . -type f \
  \( -name 'Dockerfile' -o -name 'Dockerfile.*' \
     -o -name 'Containerfile' -o -name 'Containerfile.*' \) \
  -not -path '*/.git/*' \
  -not -path '*/.ci-shared/*' \
  -not -path '*/node_modules/*' \
  -not -path '*/vendor/*' \
  -not -path '*/.venv/*' \
  -not -path '*/.worktrees/*' \
  -not -path '*/testdata/*' \
  | grep -Ev '/(Docker|Container)file\.(json|md|txt|ya?ml|lock)$' || true)

[ -n "${files}" ] || exit 0

# SORTED, so both callers see the same list in the same order — detect.sh takes
# the FIRST entry, and an unstable order would make repo-shape detection depend
# on the filesystem.
printf '%s\n' "${files}" | sort
