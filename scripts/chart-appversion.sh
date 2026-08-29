#!/usr/bin/env sh
# Verify the container image named by a chart's appVersion actually exists.
#
#   usage: chart-appversion.sh <chart-dir> <image>
#
# WHY THIS GATE EXISTS: appVersion is COMMITTED, not computed. A chart whose
# appVersion names an image that was never published installs cleanly and then
# leaves every pod in ImagePullBackOff — a failure that surfaces in the cluster,
# hours later, instead of in the pipeline that caused it. Publishing is refused
# instead.
#
# WHY A SCRIPT AND NOT AN INLINE STEP: logic a test could cover never lives in
# workflow YAML, because a shell harness cannot reach it there. This is two guards and a normalisation with two branches, and
# tests/chart-appversion-test.sh covers every one of them, which it could not
# have while the block sat inside ci-chart.yml.
#
# THE NORMALISATION RUNS THE OPPOSITE WAY FROM THE CHART TAG. Both directions are
# correct and neither may be "harmonised" with the other — see the README
# section "Two tag conventions, opposite on purpose":
#
#   chart tag  <- inputs.version   BARE semver     charts/foo:1.2.3
#   image tag  <- appVersion       V-PREFIXED      ghcr.io/owner/repo:v1.2.3
#
# Helm resolves "latest" by highest valid SemVer and SemVer forbids a leading
# `v`, so chart tags must stay bare. Images are tagged by ci-build from
# release-please's tag_name, which IS v-prefixed. appVersion is conventionally
# written bare in Chart.yaml, so the `v` is added back here — and NOT added twice
# when a repo happens to write it v-prefixed already.
#
# NO `cd` HERE, deliberately — the same trap chart-deps.sh and chart-drift.sh
# document. The chart directory arrives as an ARGUMENT and is resolved against
# $PWD, which is $GITHUB_WORKSPACE in CI; a `cd "$(dirname "$0")/.."` would
# reinterpret that argument against the script's own directory instead of the
# caller's.
#
# POSIX sh — runs identically on a runner and on macOS.
set -eu

[ $# -eq 2 ] || { echo "usage: chart-appversion.sh <chart-dir> <image>" >&2; exit 2; }
chart=$1
image=$2

# THE TOOL GUARDS ARE NOT CEREMONY. Without them an absent `docker` makes the
# inspect below fail and the script blames the IMAGE, sending whoever reads the
# log to hunt a registry problem that does not exist. Name the real cause.
command -v yq     >/dev/null || { echo "yq not installed: brew install yq" >&2; exit 1; }
command -v docker >/dev/null || { echo "docker not installed" >&2; exit 1; }

# BOTH TESTS ARE LOAD-BEARING, and they catch different spellings. Measured with
# yq v4.53.2: an ABSENT key prints the four-letter string `null`, while a present
# but null key (`appVersion:`) and an empty one (`appVersion: ""`) print nothing.
# An emptiness check alone lets the absent case through, and the ref then becomes
# a literal `<image>:vnull`.
app=$(yq '.appVersion' "${chart}/Chart.yaml")
if [ -z "${app}" ] || [ "${app}" = "null" ]; then
  echo "::error::${chart}/Chart.yaml has no appVersion" >&2
  exit 1
fi

case "${app}" in
  v*) ref="${image}:${app}" ;;
  *)  ref="${image}:v${app}" ;;
esac

echo "chart appVersion: ${app}"
echo "checking ${ref}"

# `imagetools inspect` reads the registry's manifest API — no pull, no daemon
# storage, and it understands multi-arch manifest lists, which `docker manifest
# inspect` handles less predictably.
docker buildx imagetools inspect "${ref}" >/dev/null 2>&1 || {
  echo "::error::${ref} does not exist — refusing to publish a chart pointing at a missing image" >&2
  exit 1
}

echo "ok: ${ref} exists"
