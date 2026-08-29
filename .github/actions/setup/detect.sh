#!/usr/bin/env bash
# Detect which languages and artefacts a repo contains. Prints key=value lines.
#
# Shared by action.yml (which appends this to $GITHUB_OUTPUT) and by
# tests/detect-test.sh. ONE implementation so the test cannot drift from what
# actually ships.
#
# Usage: detect.sh [DIR]   (defaults to $PWD)
#
# THREE TRAPS ARE ENCODED HERE, each of which returns a confidently empty or
# wrong answer rather than erroring:
#
#   1. `-mindepth 1` is REQUIRED. The basename of the starting point `.` matches
#      the `.*` prune glob, so without it find prunes the entire tree and prints
#      nothing — every language silently undetected, in every repo.
#
#   2. The Dockerfile extension filter is anchored to `/Dockerfile.`. A blanket
#      `\.(json|md|txt)$` also strips every package.json, so Node can never be
#      detected.
#
#   3. The caller's expression is wrapped in \( \) inside find_real. A bare `-o`
#      binds looser than `-type f` and the `-prune` clause and discards both.
#
# THE DOCKERFILE GLOB IS NOT WRITTEN HERE. It lives in scripts/dockerfile-list.sh,
# which ci-lint.yml's hadolint step reads too. Trap 1's `.*` prune is right for
# every other language marker and WRONG for Dockerfiles, and while this file
# owned its own copy of the glob the two callers disagreed about exactly that: a
# repo with .devcontainer/Dockerfile got docker=false here, so hadolint was never
# installed, while ci-lint.yml found the file anyway and died with exit 127.
set -euo pipefail

# Resolved BEFORE the cd below, which moves us into the tree under inspection.
# Fail loudly rather than reporting docker=false: a missing capability that
# answers "no Dockerfile" is the fail-silent shape this header already documents
# three instances of.
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
dockerfile_list="${here}/../../../scripts/dockerfile-list.sh"
[ -f "${dockerfile_list}" ] || {
  echo "detect.sh: cannot find ${dockerfile_list}" >&2
  exit 1
}

cd "${1:-$PWD}"

# `testdata` is in the list for a reason the other entries do not share: the Go
# toolchain itself defines it as excluded from the build, so a Go repo's
# testdata/**/package.json is a FIXTURE, not this repo's Node project. Detected
# as one, node=true made the setup action run actions/setup-node with
# `node-version-file: .node-version` — a file copier never wrote, because the
# repo answered lang_node=false — and setup-node fails outright on an absent
# version file. The whole job died before a single linter ran.
#
# The same gap existed in scripts/discover-dirs.sh, chart-list.sh and
# dockerfile-list.sh, which carry this prune list for their own callers. Keep
# the four in step.
pruned=(-name node_modules -o -name vendor -o -name .git -o -name .worktrees -o -name testdata -o -name '.*')
find_real() {
  find . -mindepth 1 \( "${pruned[@]}" \) -prune -o -type f \( "$@" \) -print 2>/dev/null
}

# SHALLOWEST path wins, not the lexically first. A repo with example or test
# modules (examples/analytics/go.mod) would otherwise beat the real ./go.mod,
# because "./e" sorts before "./g" — and go-version-file would then point at a
# nested module's Go directive.
shallowest() { awk '{ n = gsub("/", "/"); print n, $0 }' | sort -k1,1n -k2,2 | head -1 | cut -d' ' -f2-; }

gomod=$(find_real -name go.mod | shallowest || true)
node=$(find_real -name package.json | sort | head -1 || true)
lock=$(find_real -name package-lock.json | sort | head -1 || true)
py=$(find_real -name pyproject.toml -o -name requirements.txt | sort | head -1 || true)
# NOT find_real. The shared list already emits a sorted answer, and it prunes
# the vendored trees by name instead of pruning every dot-directory — which is
# what lets a .devcontainer/Dockerfile be seen here and by ci-lint.yml alike.
# Containerfile, the OCI/Podman spelling, and the anchored extension filter both
# live there; the reasons they are not optional are in that script's header.
docker=$(sh "${dockerfile_list}" | head -1 || true)
wf=$(find .github/workflows -maxdepth 1 -type f \( -name '*.yml' -o -name '*.yaml' \) 2>/dev/null | head -1 || true)

b() { [ -n "$1" ] && echo true || echo false; }

# EVERY KEY BELOW HAS A CONSUMER, and that is the rule this list is held to —
# action.yml's `outputs:` block and its five `if:` guards are the only readers,
# and tests/detect-test.sh's C1/C3 assert the two sets agree in both directions.
#
# Five keys were emitted here with no reader at all: rust, chart, chartpath,
# dockerfile and variant. Each cost something rather than merely sitting idle —
# `chart`/`chartpath` needed a THIRD copy of the Chart.yaml glob, without the
# subchart exclusion scripts/chart-list.sh exists to provide, and `dockerfile`
# carried a comment claiming ci-build reads it to decide `--file`, which it does
# not: ci-build resolves <context>/Dockerfile then <context>/Containerfile
# itself. Re-adding an output later is additive; publishing one nothing reads is
# a claim that ages into a lie.
echo "gomod=${gomod#./}"
# THE DIRECTORY, not just the file, because they have different consumers.
# setup-go takes `go-version-file: <gomod>` and sets the TOOLCHAIN — it does not
# change directory. Every Go command ci-lint.yml runs afterwards is a
# module-root operation, so each takes `working-directory: <godir>`; without it
# a module under svc/ left `go mod tidy -diff` and `go test ./...` running in the
# repo root, where they fail with `go.mod file not found` and `directory prefix
# . does not contain main module`.
echo "godir=$(dirname "${gomod:-.}" | sed 's|^\./||')"
echo "go=$(b "${gomod}")"
echo "node=$(b "${node}")"
echo "nodelock=$(b "${lock}")"
echo "python=$(b "${py}")"
echo "docker=$(b "${docker}")"
echo "workflows=$(b "${wf}")"
