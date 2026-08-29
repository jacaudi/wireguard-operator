#!/usr/bin/env sh
# Register classic (non-OCI) Helm dependency repos, then build chart
# dependencies from Chart.lock.
#
# WHY A SCRIPT AND NOT AN INLINE STEP: this is the single source of truth for
# dependency resolution, and it has THREE callers — `task chart:deps` locally,
# `task chart:lint` (which cannot template without it), and ci-chart.yml. Under
# deviation D1 CI does not read the taskfile, so an inline taskfile-only version
# would leave CI resolving dependencies differently from local. Same reasoning as
# scripts/local-refs.sh.
#
# `helm dependency build`, NOT `update`:
#   build  — installs exactly what Chart.lock pins. Reproducible.
#   update — re-resolves and REWRITES Chart.lock, so CI would silently drift the
#            lock file and publish a chart nobody tested.
#
# This is what replaces a vendored dependency tarball. Renovate can bump a
# dependency's version in Chart.yaml but cannot regenerate a committed
# charts/*.tgz, so the two drift apart silently. Commit Chart.lock, gitignore
# charts/*.tgz, and let this script rebuild them.
#
# POSIX sh — runs identically on a runner and on macOS.
set -eu

# NO `cd` HERE, deliberately. A dependency resolver must follow the CWD — which
# is $GITHUB_WORKSPACE in CI and the repo root under Task — never wherever the
# script file happens to sit. `cd "$(dirname "$0")/.."` is the defect: it
# resolves a tree the caller did not ask about and, finding no chart there,
# prints "nothing to resolve" and EXITS 0. Silently green, having done nothing —
# which is why tests/chart-deps-test.sh asserts on the chart list produced and
# not on the exit code.
#
# local-refs.sh is the deliberate exception: validating the tree that contains it
# is that gate's whole job.

# Chart discovery is single-sourced in chart-list.sh — see the trap documented there.
here=$(cd "$(dirname "$0")" && pwd)
dirs=$("${here}/chart-list.sh")
[ -n "${dirs}" ] || { echo "no Chart.yaml — nothing to resolve"; exit 0; }

# THE TOOL CHECKS COME AFTER DISCOVERY, and the order is the point.
#
# ci-lint.yml runs this stage unconditionally for every repo in the fleet,
# because "does this repo have a chart" is a question only this script can
# answer. Asserting helm and yq first meant a pure-Go library — which never
# reaches a single line below — failed lint the day the runner image stopped
# shipping either one, over tools its pipeline does not use. A stage that is
# meant to be a no-op cannot mean "a no-op, provided two unrelated binaries
# happen to be installed".
#
# Still an assertion and not a skip: once a chart IS found, a missing helm has
# to be loud. Silently declining to resolve dependencies would leave the chart
# templated, linted and published with subcharts that were never built.
command -v helm >/dev/null || { echo "helm not installed: brew install helm" >&2; exit 1; }
command -v yq   >/dev/null || { echo "yq not installed: brew install yq" >&2; exit 1; }

# Iterate WITHOUT a subshell so a `helm` failure still aborts the script under
# `set -e` — piping into `while read` would swallow it. IFS is narrowed to
# newline so a path containing spaces survives the split.
oldifs=$IFS
IFS='
'
# shellcheck disable=SC2086  # deliberate newline-only split
set -- ${dirs}
IFS=$oldifs

for d in "$@"; do
  c="${d}/Chart.yaml"
  count=$(yq '.dependencies | length // 0' "${c}")
  if [ "${count}" -eq 0 ]; then
    echo "${d}: no dependencies"
    continue
  fi

  i=0
  while [ "${i}" -lt "${count}" ]; do
    name=$(yq ".dependencies[${i}].name" "${c}")
    repo=$(yq ".dependencies[${i}].repository" "${c}")
    case "${repo}" in
      # OCI refs need no `helm repo add` — helm resolves them directly.
      oci://*|""|null) : ;;
      *) echo "helm repo add ${name} ${repo}"; helm repo add "${name}" "${repo}" >/dev/null ;;
    esac
    i=$((i + 1))
  done

  echo "helm dependency build ${d}"
  helm dependency build "${d}"
done
