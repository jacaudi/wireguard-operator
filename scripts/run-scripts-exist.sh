#!/usr/bin/env sh
# Assert every repo-relative script named in a workflow's `run:` step exists.
#
# WHY THIS EXISTS: static analysis has a hole exactly this shape. The library
# repo shipped a `run:` step invoking a script that was never copied; it passed
# actionlint, yamllint AND local-refs.sh cleanly, because local-refs.sh checks
# `uses:` and nothing checks `run:`. It failed at run time with exit 127 — on
# main, after merge. `uses: ./missing` and `run: sh missing.sh` are the same
# defect wearing different syntax, so they get the same gate.
#
# Comments are blanked first so prose mentioning a script name — including this
# header if it is ever copied into a workflow — cannot false-positive.
#
# WHAT IS DELIBERATELY NOT CHECKED, because the path is not knowable statically:
#
#   /usr/local/bin/x.sh          absolute — not this repo's to guarantee
#   ${ANY_VAR}/x.sh              an expansion — skipped rather than guessed at
#   scripts/*.sh                 a glob; `*` is outside the token class, so it
#                                never matches in the first place
#   --jq '.object.sha'           a jq FIELD PATH — see the word boundary below
#
# NO EXPANSION IS RESOLVED, and there is nothing left for one to mean: the stages
# live in the same tree as the scripts they call, so every reference is a plain
# repo-relative `sh scripts/foo.sh`.
#
# POSIX sh — runs identically on a runner and on macOS.
set -eu

[ -d .github/workflows ] || { echo "no .github/workflows — nothing to check"; exit 0; }

# `find` into a variable, NOT a bare `*.yml *.yaml` glob pair — see the trap
# documented in local-refs.sh, which this repeats deliberately rather than
# coupling the two gates together.
files=$(find .github/workflows -type f \( -name '*.yml' -o -name '*.yaml' \))
[ -n "${files}" ] || { echo "no workflow files — nothing to check"; exit 0; }

# Split on NEWLINE ONLY so a path containing spaces survives, and stay in the
# current shell so `set -e` still aborts on failure (SC2044).
oldifs=$IFS
IFS='
'
# shellcheck disable=SC2086  # deliberate newline-only split, per IFS above
set -- ${files}
IFS=$oldifs

missing=0

for wf in "$@"; do
  # `|| true` because grep exits 1 when a workflow names no script at all,
  # which is the ordinary case, not an error.
  # THE TRAILING WORD BOUNDARY IS LOAD-BEARING, exactly as local-refs.sh's
  # leading anchor is. Without it, `[token]+\.sh` matches INSIDE a longer word:
  # `--jq '.object.sha'` yields `.object.sh`, and the gate fails a correct
  # workflow over a script nobody named. ci-retag.yml resolves annotated tags
  # that way, so this was not hypothetical. A gate that cries wolf gets
  # disabled, which costs more than the defect it was catching.
  #
  # `.sh` must therefore be followed by a NON-token character or end-of-line.
  # grep has no lookahead, so that character is captured and then stripped —
  # a quote, a space, a `&`. It cannot hide a second reference: grep -o
  # resumes scanning immediately after each match.
  #
  # shellcheck disable=SC2016  # the '${}' below are CHARACTER CLASS members, so
  #                            an expansion is matched as literal workflow text.
  #                            Do not requote.
  refs=$(sed 's/[[:space:]]*#.*$//' "${wf}" \
           | grep -oE '[A-Za-z0-9_./${}-]+\.sh([^A-Za-z0-9_./${}-]|$)' \
           | sed 's|[^A-Za-z0-9_./${}-]$||' \
           | sort -u || true)
  [ -n "${refs}" ] || continue

  # Safe to word-split: the token class above admits no whitespace.
  for r in ${refs}; do
    case "${r}" in
      /*) continue ;;
      *) p=${r} ;;
    esac
    # An expansion makes the path unresolvable — skip it, do not guess.
    case "${p}" in *'$'*) continue ;; esac

    if [ ! -f "${p}" ]; then
      echo "::error file=${wf}::run: references '${r}' which does not exist at '${p}'" >&2
      missing=1
    fi
  done
done

[ "${missing}" = 0 ] || exit 1
echo "run-scripts-exist: ok"
