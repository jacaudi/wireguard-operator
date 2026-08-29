#!/usr/bin/env sh
# Regenerate the chart from its hack/ generators, then FAIL on any diff.
#
# Operator charts derive their CRDs and RBAC from config/, so a committed chart
# can silently fall behind the API types. This regenerates and then fails —
# it never fixes the drift, because a CI step that mutates the tree hides the
# problem it exists to surface.
#
# WHY A SCRIPT AND NOT AN INLINE STEP: two reasons, and the second is the one
# that forced it.
#
#   1. Logic a test could cover lives here, never in workflow YAML. This
#      is three branches and a git verdict; tests/chart-drift-test.sh covers all
#      of them, and could not have if it stayed inline.
#   2. scripts/run-scripts-exist.sh resolves a workflow's `run:` script paths
#      against THIS repo, and correctly reported `hack/sync-helm-crds.sh` as
#      missing when this block was inline in ci-lint.yml. It is not wrong: the
#      generators belong to the CONSUMER's workspace and this repo has no hack/.
#      Naming them here — in a script, where the gate does not look, because a
#      script is not a workflow — states that ownership instead of suppressing
#      the gate.
#
# NO `cd` HERE, deliberately — the same trap chart-deps.sh documents. A drift
# gate that resolved its own directory instead of the CWD would regenerate and
# diff whichever tree the script file sits in: always clean, always green,
# checking nothing. It operates on $PWD, which is $GITHUB_WORKSPACE in CI and the
# repo root under Task.
#
# The generators are OPTIONAL and named exactly, not globbed: `hack/` in an
# operator repo also holds install-tools.sh and friends, and running everything
# in it would execute arbitrary consumer scripts. A repo with neither generator
# has a hand-maintained chart and is skipped.
#
# POSIX sh — runs identically on a runner and on macOS.
set -eu

ran=0
for g in hack/sync-helm-crds.sh hack/generate-helm-rbac.sh; do
  # -x, not -f: a generator committed without its exec bit cannot be run, and
  # `./x` would die with a permission error instead of skipping.
  [ -x "${g}" ] || continue
  "./${g}"
  ran=1
done

if [ "${ran}" = 0 ]; then
  echo "chart is hand-maintained (no hack/ generators) — skipping drift gate"
  exit 0
fi

# `git status`, NOT `git diff`. THE VERDICT MUST INCLUDE UNTRACKED FILES.
#
# `git diff --quiet -- charts` compares TRACKED files only, and the ordinary way
# a chart falls behind its API types is a file that does not exist yet:
# controller-gen emits ONE FILE PER CRD, so adding a type adds a file. Left
# untracked, it is invisible to `git diff`, the gate exits 0, and the chart
# publishes without the CRD — `helm install` succeeds and the operator fails in
# the cluster, hours later. Measured on a scratch repo: after
# `printf 'new: crd\n' > charts/crds-brand-new.yaml`, `git diff --quiet -- charts`
# exits 0 while `git status --porcelain -- charts` prints `?? charts/…`.
#
# `--untracked-files=all` lists the individual files rather than collapsing a
# new directory to one entry, so the diagnostic names what actually appeared.
#
# THIS IS ONE MECHANISM WITH .gitignore, not two. Everything the gate tolerates
# is what .gitignore excludes — and in CI chart-deps.sh has already run
# `helm dependency build charts`, which writes charts/charts/<dep>.tgz. That
# path is NOT matched by `charts/*.tgz`, so `.gitignore` carries `**/charts/*.tgz`
# alongside it — verified against the shipped .gitignore, which has both. Narrowing one without the other fails every chart repo
# that has a dependency. tests/chart-drift-test.sh covers both directions with
# THIS repo's .gitignore copied into the fixture, so the pair cannot drift.
#
# `-- charts` is plural and hardcoded: one sanctioned layout fleet-wide.
# An unmatched pathspec exits 0, so a repo still using `chart/` would pass this
# gate while checking nothing — which is why `chart-path` is not an input.
drift=$(git status --porcelain --untracked-files=all -- charts)
if [ -n "${drift}" ]; then
  echo "::error::chart is stale — run: task chart:generate"
  printf '%s\n' "${drift}"
  exit 1
fi

echo "chart is up to date with config/"
