#!/usr/bin/env sh
# Assert that no `run:` block interpolates a GitHub expression.
#
# WHY THIS EXISTS: ci-lint.yml already states the rule — "a published
# `workflow_call` input must never be pasted straight into a shell command" —
# and every stage but one obeyed it. A rule with no gate is a rule that holds
# until someone is in a hurry, which is what happened in ci-retag.yml:
#
#   for t in ${{ steps.tags.outputs.list }}; do
#
# GitHub substitutes expressions into the run: script as TEXT, before any shell
# sees it, so whatever the expression contains becomes shell source. That list
# is derived from a `workflow_call` input a consumer supplies, and the shape
# check it passed through was `case "${v}" in *.*.*)`, which `v1.2.3$(id -un)`
# satisfies. The correct form is an `env:` entry and a "${VAR}" reference, which
# reaches the shell as DATA.
#
# Note that even a trusted expression belongs in `env:`: an image digest or an
# action path is not attacker-controlled, but the reviewer of the next diff
# cannot tell the safe interpolations from the unsafe ones at a glance, and a
# rule with exceptions is a rule nobody applies.
#
# COMPOSITE ACTIONS ARE INCLUDED. actionlint cannot lint them at all (see
# self-test.yml), so for `.github/actions/*/action.yml` this is the only static
# check there is.
#
# yq, NOT grep: the question is "is this expression inside a run: value", and
# answering it by hand means re-implementing YAML block-scalar parsing. `run:`
# steps also sit at different depths in a workflow and in a composite action, so
# the recursive descent below covers both without an allowlist of paths. A
# comment inside a run: block is deliberately still matched — GitHub expands
# expressions across the whole block, comments included, which ci-retag.yml
# documents having been bitten by.
#
# NO `cd` HERE. It operates on $PWD, the way run-scripts-exist.sh does, which is
# what lets tests/gates-test.sh point it at a fixture tree.
#
# POSIX sh — runs identically on a runner and on macOS.
set -eu

dirs=''
if [ -d .github/workflows ]; then dirs="${dirs} .github/workflows"; fi
if [ -d .github/actions ];   then dirs="${dirs} .github/actions"; fi
[ -n "${dirs}" ] || { echo "no workflows or composite actions — nothing to check"; exit 0; }

# shellcheck disable=SC2086  # deliberate word split over the directory list
files=$(find ${dirs} -type f \( -name '*.yml' -o -name '*.yaml' \))
[ -n "${files}" ] || { echo "no workflow files — nothing to check"; exit 0; }

# Fail loudly rather than checking nothing. yq ships on ubuntu-latest.
command -v yq >/dev/null || { echo "yq not installed: brew install yq" >&2; exit 1; }

# Split on NEWLINE ONLY so a path containing spaces survives, and stay in the
# current shell so `set -e` still aborts on failure (SC2044).
oldifs=$IFS
IFS='
'
# shellcheck disable=SC2086  # deliberate newline-only split, per IFS above
set -- ${files}
IFS=$oldifs

bad=0

for wf in "$@"; do
  # Every `run:` value in the document, however deeply nested. The `!!map` guard
  # is required: `has()` errors out on a scalar, and `..` visits every scalar.
  runs=$(yq -r '.. | select(type == "!!map" and has("run")) | .run' "${wf}")
  [ -n "${runs}" ] || continue

  # `[$]` is a bracket expression, not an escape. It keeps the dollar literal
  # for grep AND out of shellcheck's SC2016 heuristic.
  hits=$(printf '%s\n' "${runs}" | grep '[$]{{' || true)
  [ -n "${hits}" ] || continue

  echo "::error file=${wf}::a run: block interpolates an expression — pass it through env: instead" >&2
  printf '%s\n' "${hits}" >&2
  bad=1
done

[ "${bad}" = 0 ] || exit 1
echo "run-interpolation: ok"
