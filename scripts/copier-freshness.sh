#!/usr/bin/env sh
# Report whether this repo's copier-generated files are behind the template.
#
# WHY THIS EXISTS: every file here is COPIED — the stages, the composite action,
# these scripts, and every config a tool reads. That is what makes a run
# reproducible from one commit, and it is also what lets a repo fall arbitrarily
# far behind the template with nothing anywhere saying so.
#
# copier records in .copier-answers.yml the template version a repo generated
# from; this stage runs in every repo. Neither half can detect the drift alone.
#
# WARNS, NEVER FAILS. A hard failure would turn every repo red the moment the
# template moves — reintroducing exactly the fleet-wide blast radius this design
# works to avoid. The gap being closed is "undetected", not "unblocked". Every
# path out of this script exits 0, deliberately.
#
# POSIX sh — runs identically on a runner and on macOS. Operates on $PWD.
set -eu

ANSWERS=.copier-answers.yml
[ -f "${ANSWERS}" ] || { echo "no ${ANSWERS} — repo is not copier-managed, skipping"; exit 0; }

have=$(grep '^_commit:' "${ANSWERS}" | head -1 | sed 's/^_commit:[[:space:]]*//; s/["'"'"']//g')
src=$(grep '^_src_path:' "${ANSWERS}" | head -1 | sed 's/^_src_path:[[:space:]]*//; s/["'"'"']//g')
[ -n "${have}" ] || { echo "::warning::${ANSWERS} has no _commit — cannot check freshness"; exit 0; }

# Resolve owner/repo from a gh: shorthand or an https URL; anything else (a local
# path, as used while developing the template) is unresolvable and skipped.
case "${src}" in
  gh:*)                 slug=${src#gh:} ;;
  https://github.com/*) slug=${src#https://github.com/}; slug=${slug%.git} ;;
  *) echo "template source '${src}' is not a GitHub repo — skipping freshness check"; exit 0 ;;
esac

# `|| true` covers every way this can fail — no network, no auth, no releases,
# no `gh` on PATH — because none of them is this repo's problem to fail on.
latest=$(gh api "repos/${slug}/releases/latest" --jq .tag_name 2>/dev/null || true)
if [ -z "${latest}" ]; then
  echo "::warning::could not resolve the latest release of ${slug} — freshness unknown"
  exit 0
fi

if [ "${have}" = "${latest}" ]; then
  echo "copier template ${slug} is current (${have})"
else
  echo "::warning file=${ANSWERS}::template ${slug} is at ${latest}, this repo was generated from ${have}. Run: uvx copier update --trust"
fi
