#!/usr/bin/env sh
# Assert every local `uses:` reference in .github/workflows/ points at something
# that actually exists.
#
# WHY THIS EXISTS: actionlint has a verified blind spot, but a NARROWER one
# than this comment used to claim. Measured against v1.7.12:
#
#   uses: ./.github/workflows/missing.yml   -> actionlint DOES catch it
#         ("could not read reusable workflow file", exit 1)
#   uses: ./.github/actions/missing         -> actionlint says NOTHING, exit 0
#
# So the composite-action case is the real gap, and it is the one that bites:
# a consumer that excludes a stage but keeps its `uses:` gets caught by
# actionlint, while a typo'd action path ships silently. A bogus *input* to a
# local composite action IS caught, but `uses: ./path` where the path does not
# exist is SILENT. Every stage in this pipeline is a local reference, so a stage
# file or .github/actions/setup/ the template did not write passes lint cleanly
# and fails only at run time — on main, after merge.
#
# THIS IS THE GATE ON THE TEMPLATE'S PER-VARIANT EXCLUDES. A library repo gets no
# ci-build.yml; a repo with no chart gets no ci-chart.yml. Should ci.yaml ever
# call a stage its own variant excluded, nothing else in the tree notices — not
# actionlint, not yamllint. This does.
#
# Called by ci-lint.yml and by `task repo:actions:local-refs`, so CI and local
# run the one copy.
#
# POSIX sh, no bashisms — runs identically on a runner and on macOS.
set -eu

# "Validate the tree that contains me" is this gate's whole job, so it resolves
# its own root rather than trusting the CWD — it behaves identically run from the
# repo root or from a subdirectory, and tests/gates-test.sh copies it into a
# scratch tree precisely because of this line, which is what pins the behaviour.
#
# Do NOT copy it into a script that operates on the workspace: chart-deps.sh had
# to lose exactly this line, because a chart resolver must follow the CWD.
cd "$(dirname "$0")/.."

[ -d .github/workflows ] || { echo "no .github/workflows — nothing to check"; exit 0; }

# `find -print` into a variable, NOT a bare `*.yml *.yaml` glob pair: when one
# glob has no match, awk dies on the literal unexpanded argument, 2>/dev/null
# hides the error, and the pipe swallows the exit status — so an all-.yaml repo
# silently "passed" while checking nothing.
files=$(find .github/workflows -type f \( -name '*.yml' -o -name '*.yaml' \))
[ -n "${files}" ] || { echo "no workflow files — nothing to check"; exit 0; }

fail=0

# THE MATCH IS ANCHORED AT LINE START, and this is load-bearing.
#
# An optional leading `#` is allowed, so a COMMENTED-OUT directive is still
# checked — a commented reference to a stage file you never copied is a live
# landmine the moment someone uncomments it. If you do not ship the stage,
# DELETE the lines rather than commenting them.
#
# But the match must not be a bare substring search. An unanchored version
# matches PROSE that merely mentions the string — e.g. this file's own comment
# above, "a bogus `uses: ./path` where the path does not exist" — and then emits
# `./path``, `where`, and `the` as missing actions. That is not hypothetical:
# it is what the unanchored version did when first run against this tree.
#
# Anchoring to (indent)(optional #)(optional "- ")uses: admits real YAML,
# commented or not, and rejects prose, where `uses:` is always preceded by other
# words or a backtick.
# shellcheck disable=SC2086  # deliberate word split over the file list
refs=$(awk '
  /^[[:space:]]*#?[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*\.\// {
    sub(/^[[:space:]]*#?[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*/, "")
    sub(/[[:space:]].*$/, "")   # first token only — drop trailing comments
    print
  }' ${files} | sort -u)

for ref in ${refs}; do
  case "${ref}" in
    *.yml|*.yaml)
      # A reusable workflow — must be a file.
      [ -f "${ref#./}" ] || { echo "MISSING workflow: ${ref}"; fail=1; }
      ;;
    *)
      # A composite action — must be a DIRECTORY containing action.yml.
      d="${ref#./}"
      if [ ! -f "${d}/action.yml" ] && [ ! -f "${d}/action.yaml" ]; then
        echo "MISSING action:   ${ref} (no action.yml)"
        fail=1
      fi
      ;;
  esac
done

if [ "${fail}" != "0" ]; then
  echo "::error::a local uses: reference points at nothing"
  exit 1
fi

echo "local uses: references ok"
