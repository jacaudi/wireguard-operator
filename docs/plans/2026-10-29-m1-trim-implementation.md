# M1 (trim) Implementation Plan

**Spec:** docs/designs/2026-10-29-minimal-vpn-gateway-design.md

**Goal:** Milestone M1 of the minimal VPN gateway design: delete every external-client feature and everything design §2.2 lists, add `iproute2` to the agent image, and leave a `Wireguard` that still yields a ClusterIP server whose `publicKey` peers still get addresses (in `spec.address`, as today).

**Architecture:** A trim of the existing kubebuilder operator, in ten dependency-ordered tasks. Each task deletes one feature end to end (API field, builder, controller branch, agent code, tests, examples, generated CRDs/RBAC/deepcopy, `release.yaml`) and leaves the tree compiling, formatted, and with every remaining test green. All source edits ship as verified `git apply` patches embedded in this file and extracted with `awk`, so no builder retypes a patch; whole Ginkgo blocks are removed with anchored `perl` range deletes.

**Tech Stack:** Go 1.27.2 (local), controller-runtime/envtest (Kubernetes 1.30.0 assets), controller-gen v0.16.5 and kustomize v5.4.2 via the Makefile, Ginkgo v1, golangci-lint v2 config in `.golangci.yml`.

> **For Claude:** REQUIRED EXECUTION WORKFLOW (follow in order):
> 1. `superpowers:using-git-worktrees` — the worktree already exists; verify it, never create a second
> 2. `superpowers:subagent-driven-development` — one builder per task (batched where SDD says so), a fresh critic per task
> 3. `superpowers:test-driven-development` — every builder
> 4. `superpowers:verification-before-completion` — every task
> 5. Per-task review is SDD's own `task-reviewer-prompt.md` (built in)
> 6. After all tasks: whole-branch review by `sr-eng-review` (automatic; see the feature-workflow skill, Phase 4)
> 7. `superpowers:finishing-a-development-branch` — executes the run contract
>
> Skills carry their own model and effort settings. Do not override them; the feature-workflow skill's routing table is the only place a model is chosen.

**Run contract:** (answered by the repo owner 2026-10-29; reused from issue #2's run with "merge on green"; recorded verbatim)
- **Integration when done:** (b) push the branch and open a **ready** (not draft) PR to `main` on `jacaudi/wireguard-operator` with `gh`; once every CI check is green, rebase-merge it (`gh pr merge --rebase --match-head-commit <head>`). Never via the GitHub MCP write tools. *Amended by controller ruling (2026-10-29):* the finisher waits by dispatching the `ci-watch` skill with `--merge` and `--sha <head>`. The owner's recorded answer (merge on green) is the `--merge` authorisation for ci-watch. If any required check fails, the outcome is `PLAN: BLOCKED — every path is a guess: finish: CI red`, with the failing check named in the handoff. No fix-forward on main.
- **Base branch:** `main`.
- **Remote and forge:** `git@github.com:jacaudi/wireguard-operator.git`, using `gh`.
- **Repository owned by the user:** yes (`jacaudi/`), so option (b) and the merge are authorized for this branch only.
- **PR details:** ready. Title `<type>(<scope>): <description>` per the owner's `pr` skill. The body uses that skill's template (Summary, Evidence, Merge Danger) plus a collapsible run record listing every ruling. No labels and no reviewers. *Amended by controller ruling (2026-10-29):* no M1 milestone issue exists and none is created; the body ends with `Closes #50`, `Closes #51` (M1 removes `spec.metric` in Task 2 and the ConfigMap/`envFrom` in Task 7) and then `Refs #37`. Merge Danger states: peer internet egress is broken from M1 until M4 restores it via gluetun (Task 1 removes the MASQUERADE); nothing is released.
- **Direct push to the base branch:** never.
- **Branch name:** `feat/minimal-vpn-gateway` (the existing worktree branch; it also carries the approved design commit).
- **Delivery:** one PR per milestone (owner, 2026-10-29). This plan covers milestone M1 only; M2 starts from the merged `main`.
- **Sanctioned test command:** `go build ./... && go vet ./...`, `go test ./... -race`, and the integration suite `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race`. Lint via golangci-lint gated on the printed `0 issues.` line (see Global Constraints for the local go1.27.1 toolchain workaround).
- **Sanctioned environment:** the host toolchain (the repo has no `.devcontainer/`).
- **Commit types:** a user-visible behaviour or API change is `feat:`/`fix:` (with `!` only if the owner approves a breaking marker; v1alpha1 breaking changes are allowed because nothing was ever released). Pure deletions of internal code are `refactor:`; tests are `test:`; docs are `docs:`. *M1 ruling (owner, 2026-10-29):* commits that remove user-facing CRD fields (spec/status fields, Services, ports, features users configure) are `refactor!:`, so release-please cuts 3.0.0 with a BREAKING CHANGES section; internal-only deletions stay `refactor:`, docs `docs:`, the image change `build(agent):`.
- **Commits are signed** (1Password SSH). Never `--no-gpg-sign`. A signing failure or hang is `PLAN: BLOCKED`.
- **New dependencies:** allowed only with a present requirement, pinned, and named in the task report.
- **Boundaries:** 8 hours of wall clock. A dispatch cap of 6 × tasks + 6. 2 hours with no task completing.
- **Refusals:** on a safety-classifier refusal, retry that one dispatch once on Sonnet with the same brief and ledger it; a second refusal on the same step is `PLAN: BLOCKED — security-sensitive: refusal`.
- **Critical-thinking MCP tool:** not used in this run (owner ruling, 2026-10-28).
- **Worktree after the merge:** keep it until the owner asks to remove it.

---

## Divergences accepted against the spec (controller ruling)

The design (`b2f0353`) is committed and approved and is not edited. Where this plan departs from it, the controller has ruled for the plan (2026-10-29). Each departure below carries its ruling; the SDD controller applies them as settled and does not re-rule.

1. Ruling: accepted. **`WireguardPeerReconciler` is kept in M1, minus key minting.** §2.2 lists it under Deleted, but its remaining duties (Pending status, the duplicate-address check, and the server ownerReference that makes `Owns(&WireguardPeer{})` deliver peer events to the `Wireguard` controller) are only absorbed in M2. Deleting it now would stop new peers from triggering a server reconcile.
2. Ruling: accepted. **The sysctl init container becomes unconditional in M1 (Task 7), not M4.** Deleting `enableIpForwardOnPodInit` (§2.2: "forwarding is unconditional") leaves two choices: always or never. "Never" leaves `ipForwardInitContainer` dead (the `unused` linter fails) and silently drops forwarding.
3. Ruling: accepted. **The agent's `--wg-use-userspace-implementation` flag and `SyncLink`'s forced-userspace branch go with the CRD knob.** §2.2 names only the CRD field; with it gone the flag has no setter. The automatic kernel→userspace fallback stays, and it inherits the deleted branch's bounded 5 s wait for the link (`waitForLink`), so a node without the kernel module still syncs on the first pass.
4. Ruling: accepted. **`internal/it/it_test.go` is trimmed, not deleted.** §5 M1 deletes "its metrics spec", and M3 needs the surviving "server starts" spec. The two wstunnel specs go with wstunnel. The "server starts" spec drops `mtu`/`serviceType` and gives its peer a `publicKey`. The manager-metrics spec stays, because the manager's own metrics endpoint is not the agent exporter §2.2 deletes.
5. Ruling: accepted. **The Makefile has no `bundle*` targets**, only the `VERSION`, `CHANNELS`, `DEFAULT_CHANNEL`, `BUNDLE_*` and `IMAGE_TAG_BASE` variables. Those go, together with `PROJECT`'s two operator-sdk plugins (`manifests`, `scorecard`).
6. Ruling: accepted. **`prometheus/client_golang` cannot leave `go.mod`**: controller-runtime requires it, so it becomes `// indirect`.
7. Ruling: accepted. **Deleted although §2.2 does not name them:** `grafana-dashboard.json` and `readme/screenshot.png` (the dashboard charts the deleted exporter); `config/default/metrics_reader_patch.yaml` and the commented `[PROMETHEUS]` blocks (they only serve `config/prometheus/`).
8. Ruling: accepted. **Task order is by dependency, not table order.** IPv6 goes before peer-config rendering so that the trimmed `updateWireguardPeers` stays under gocyclo's 15 (the other order measured 17).
9. Ruling: accepted. **Two envtest specs change mechanism, not intent.** "reports Degraded…" forces a permanent apply failure with a memory request above its limit (reason `DeploymentApplyFailed`), because `nodePort` no longer exists. "adopts a Deployment written before Server-Side Apply…" uses a legacy-owned toleration instead of the wstunnel sidecar.
10. Ruling: accepted. **`status.address/port/uniqueIdentifier/resources` are deleted with no replacement until M2.** §2.2 says "replaced by `status.endpoint`, `status.publicKey`", but those are M2 fields. `status.status`/`status.message` and their print column stay, because §2.2 does not list them and the e2e wait reads them.
11. Ruling: accepted. **A peer without a `publicKey` is not allocated an address and reports `Ready=False`, reason `PublicKeyMissing`** (Task 5), instead of being marked Ready while never configured. When it later gains a key it is allocated an address, set to `ready`, and the `Ready` condition is removed, so no stale `PublicKeyMissing` is left behind. This is a small behaviour addition beyond a pure trim, required by the plan gate (pass 1 S1, pass 2 S-A).
12. Ruling: accepted. **Peer internet egress is broken from M1 until M4.** Task 1 removes the only MASQUERADE; M4 restores egress through gluetun. Nothing is released, so no user is affected; the PR's Merge Danger section says so.
13. Ruling (owner, 2026-10-29). **Commit types.** Every commit that removes user-facing CRD fields or features users configure is `refactor!:` with a `BREAKING CHANGE:` footer, so release-please cuts 3.0.0 with a BREAKING CHANGES section. Task 1 is `refactor(agent)!:` because it removes the NAT egress and egress-policy enforcement users configured. Internal-only deletions (Task 8) stay `refactor:`; docs are `docs:`; the image change is `build(agent):`.

## Global Constraints

Every task's requirements implicitly include this section.

- Scope is M1 only (design §5): no `status.address` on `WireguardPeer`, no `Wireguard.status.{endpoint,publicKey}`, no gluetun, no `spec.localCIDRs`, no new controller shape. Peers keep their allocated address in `spec.address` until M2.
- Module path `github.com/jacaudi/wireguard-operator`, API group `vpn.wireguard-operator.io`, kinds `Wireguard` and `WireguardPeer`, version `v1alpha1`: unchanged (design §2).
- "Breaking `v1alpha1` in place is acceptable: the fork has published no release and no chart" (design §2). No conversion, no migration code.
- Worktree: `/Users/acaudill/Projects/github/wireguard-operator/.worktrees/feat/minimal-vpn-gateway`, branch `feat/minimal-vpn-gateway`. Its base is `b2f0353` (the signed design commit, parent `929e0fe` = `origin/main`); this plan is committed on top as `docs: add M1 trim implementation plan`. Every command runs from the worktree root.
- Apply every patch and create every new file only through the `awk … docs/plans/2026-10-29-m1-trim-implementation.md` command its step gives. Never retype a patch. If `git apply` reports any error, the tree is not in the state the plan assumes: use the restart below; if the same patch fails again, report `PLAN: BLOCKED — every path is a guess: plan drift <task>/<step>`. "Never hand-edit" applies to the patch steps only: edits a per-task critic asks for in a fix round, after the patch steps, are ordinary edits and go in their own commit.
- **Restart inside a task (authorized for a fresh builder or a resumed controller):** `git reset --hard <previous task's commit> && git clean -fd -- api cmd config examples internal images`, then re-run the task from its Step 1. `<previous task's commit>` is `git log -1 --format=%h` before the task started (for Task 1, `b2f0353` plus the plan commit, i.e. the plan commit's hash). Nothing else may be reset.
- Generated files are never hand-edited: `api/v1alpha1/zz_generated.deepcopy.go`, `config/crd/bases/*`, `config/rbac/role.yaml` come from `make manifests generate`; `release.yaml` comes from `make generate-release-file` with the image pins read back out of `release.yaml` (exact block in each task). Commit them with the task that changes their source.
- Lint: the local golangci-lint was built with go1.27.1 and fails on go1.27.2 with `export data version 5` typecheck errors. Always run `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...` and gate on the printed `0 issues.` line, never the exit status. The config's `new-from-merge-base: origin/main` ratchet only reports issues on lines this branch changed, and it does not see formatting.
- Format: CI's required `ci` check also runs `golangci-lint fmt --diff`. Every task runs `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint fmt --diff; echo fmt-exit=$?` and expects only `fmt-exit=0`.
- Any scratch directory or throwaway worktree goes under `/private/tmp`, never `/tmp` (the lint ratchet falsely reports `0 issues.` there).
- Vulnerability scan: locally under go1.27.2 govulncheck reports GO-2026-6809, which is the local toolchain and expected. `GOTOOLCHAIN=go1.26.10 govulncheck ./...` must exit 0.
- Envtest assets: `setup-envtest use 1.30.0 -p path` (the Makefile's `ENVTEST_K8S_VERSION`).
- Every commit is signed (1Password SSH). Verify with `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'` expecting `1`; do not use `%G?` (prints `N` here because no allowed-signers file is configured). A signing failure or hang is `PLAN: BLOCKED`. Never `--no-gpg-sign`, never `--no-verify`, never push (finishing does that).
- Commit messages: the exact subject, body and `BREAKING CHANGE:` footer each task gives, plus the `Co-Authored-By` trailer your harness specifies. Never a `claude.ai/code/session_` URL.
- No new dependencies. `go.mod` changes only as Task 2's patch states (`prometheus/client_golang` becomes `// indirect`).
- No Python is needed. If you reach for it anyway, use `uvx python`, never `python`/`python3`/`pip`.
- The shell is zsh: quote globs (`--include='*.go'`).
- The RTK hook may compact `grep`/`go test` output and swallow exit statuses. Every check below prints its own verdict (`grep-exit=…`, `0 issues.`, `fmt-exit=0`, a count); if output looks summarised, rerun the same command prefixed with `rtk proxy`.
- Stop labels use the run's classes, e.g. `PLAN: BLOCKED — every path is a guess: plan drift <task>/<step>`.

## Review Focus

Inputs and conditions the design implies but its own tests do not exercise, most likely to bite first. Each one's test lives in the task that owns the code.

1. **The agent reads a `state.json` written by the pre-trim operator** (during a rollout, the Secret still carries `mtu`, `serviceType`, `status.address`, `status.dns`, `privateKeyRef`). Expected: unknown fields are ignored and the state validates. Test: `TestIsStateValidAcceptsAStateWrittenBeforeTheTrim` (Task 5).
2. **A `WireguardPeer` with no `publicKey`** (the old README example). Expected: no key is minted, no `<peer>-peer` Secret appears, no address is allocated, the peer reports `Ready=False` reason `PublicKeyMissing` with `status.status` `pending`, and it never reaches the server's `state.json`. Once it is given a key it gets an address, `status.status` `ready`, and no `Ready` condition remains. Test: the keyless peer inside "allocates an address to a publicKey peer and writes no client config" (Task 5).
3. **A cluster still holding `<name>-svc` as a `LoadBalancer` from the pre-trim operator.** Expected: the Service converges to `ClusterIP` and the `Wireguard` reaches Ready, not a wedged Degraded. Test: "converts a Service left as LoadBalancer by an earlier operator to ClusterIP" (Task 6).
4. **Objects the pre-trim operator created and the trimmed one no longer manages** (`<name>-metrics-svc`, `<name>-config`, `<name>-peer-configs`, `<peer>-peer`). Expected: left alone (garbage-collected with their owning `Wireguard` through the ownerReferences they already carry), and no code path reads them. Test: the leftover-name grep step in Task 7.
5. **A `WireguardPeer` whose `publicKey` is not a valid WireGuard key.** Today `createPeersConfiguration` returns the `wgtypes.ParseKey` error and the agent configures no peer at all. M1 raises the exposure, because keys are now always user-supplied. No test in M1: pinning the expected behaviour needs new validation; it is parked for M2's §3.2 validations.

## File map

| Path | Tasks | Responsibility after M1 |
|---|---|---|
| `api/v1alpha1/wireguard_types.go` | 2,3,4,5,6,7 | `WireguardSpec{PeerCIDR, NodeSelector, Tolerations, Agent}`, `WireguardStatus{Status, Message, Conditions}` |
| `api/v1alpha1/wireguardpeer_types.go` | 4,5 | `WireguardPeerSpec{Address, PublicKey, WireguardRef}` |
| `internal/controller/wireguard_controller.go` | 2–7 | ClusterIP Service, server key Secret + `state.json`, Deployment, address allocation for keyed peers |
| `internal/controller/wireguardpeer_controller.go` | 4,5 | Pending/Error status, duplicate-address check, server ownerRef (no key minting) |
| `internal/resources/{deployment,service,secret}.go` | 2,3,5,6,7 | builders; `configmap.go` deleted |
| `internal/agent/agent.go` | 4,5 | state watcher; `IsStateValid` checks only the private key and per-peer address/publicKey |
| `internal/wireguard/wireguard.go` | 4,7 | IPv4 link/peer sync with automatic, bounded userspace fallback |
| `cmd/agent/main.go` | 1,2,7 | agent without iptables, metrics or the forced-userspace flag |
| `internal/iptables/`, `internal/agent/wireguard_metrics.go`, `bundle/`, `config/{prometheus,manifests,scorecard}/` | 1,2,8 | deleted |
| `images/agent/Dockerfile` | 2,10 | debian:trixie with `iproute2 iptables wireguard-tools` |

## Pre-flight (run once, before Task 1)

Run: `git rev-parse --abbrev-ref HEAD && git merge-base --is-ancestor b2f0353 HEAD && echo based-on-b2f0353`
Expected: `feat/minimal-vpn-gateway` then `based-on-b2f0353`.

Run: `git status --porcelain`
Expected: no output. This plan must already be committed (`docs: add M1 trim implementation plan`): a file still staged here would ride into Task 1's commit. If anything is listed, stop: `PLAN: BLOCKED — every path is a guess: pre-flight`.

Run: `git diff --stat b2f0353 HEAD -- . ':!docs'`
Expected: no output (nothing but docs has changed since `b2f0353`, so every patch below applies exactly).

Run: `test -f docs/plans/2026-10-29-m1-trim-implementation.md && echo plan-present`
Expected: `plan-present` (every patch step reads its patch out of this file).

Run: `setup-envtest use 1.30.0 -p path; echo; command -v golangci-lint govulncheck yamllint hadolint perl awk`
Expected: a path ending in `k8s/1.30.0-darwin-arm64`, then one path per tool. A missing tool is `PLAN: BLOCKED — every path is a guess: pre-flight`.

Run: `make controller-gen kustomize > /dev/null && test -x bin/controller-gen && test -x bin/kustomize && echo tools-ok`
Expected: `tools-ok` (fetches the pinned codegen tools into `bin/` up front, so the first regenerate block does not download). A failure is `PLAN: BLOCKED — every path is a guess: pre-flight`.

---

### Task 1: Delete the server iptables path

Kind: refactor

**Why first:** `Iptables.Sync` runs `iptables-restore` without `--noflush` and masquerades `-o eth0` (design §2.2). Left past M1 it would wipe gluetun's rules and leak, so it goes before anything else.

**Files:**
- Delete: `internal/iptables/iptables.go`, `internal/iptables/iptables_test.go`
- Modify: `cmd/agent/main.go` (drop the import, the `iptables.Iptables` value and the `it.Sync` call)

**Interfaces:**
- Consumes: nothing.
- Produces: the agent binary no longer imports `internal/iptables`. `api/v1alpha1` peer policy types become unused here and are deleted in Task 4.

This is a pure deletion: no new behaviour to drive out, so there is no new failing test (CONTRIBUTING.md, "TDD" on refactors: green baseline, green after). The deleted package's own tests go with it.

- [ ] **Step 1: Green baseline**

Run: `go build ./... && go test ./... -count=1`
Expected: no `FAIL`.

- [ ] **Step 2: Delete the package**

```bash
git rm -rq internal/iptables
```

- [ ] **Step 3: Remove the call site**

```bash
awk -v id=t1-impl '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t1-impl -->
~~~~diff
diff --git a/cmd/agent/main.go b/cmd/agent/main.go
index f2e8bfa..c753e24 100644
--- a/cmd/agent/main.go
+++ b/cmd/agent/main.go
@@ -13,7 +13,6 @@ import (
 
 	"github.com/go-logr/stdr"
 	"github.com/jacaudi/wireguard-operator/internal/agent"
-	"github.com/jacaudi/wireguard-operator/internal/iptables"
 	"github.com/jacaudi/wireguard-operator/internal/wireguard"
 )
 
@@ -81,9 +80,6 @@ func main() {
 		WgUserspaceImplementationFallback: wgUserspaceImplementationFallback,
 		WgUseUserspaceImpl:                wgUseUserspaceImpl,
 	}
-	it := iptables.Iptables{
-		Logger: log.WithName("iptables"),
-	}
 
 	close, err := agent.OnStateChange(configFilePath, log.WithName("onStateChange"), func(state agent.State) {
 		log.Info("Received a new state")
@@ -93,12 +89,6 @@ func main() {
 		if err != nil {
 			log.Error(err, "Error while sycncing wireguard")
 		}
-
-		err = it.Sync(state)
-		if err != nil {
-			log.Error(err, "Error while syncing network policies")
-		}
-
 	})
 
 	if err != nil {
~~~~

- [ ] **Step 4: Nothing still drives iptables-restore**

Run: `grep -rn 'iptables-restore\|internal/iptables' --include='*.go' . ; echo grep-exit=$?`
Expected: only `grep-exit=1`.

- [ ] **Step 5: Build and vet every build tag**

Run: `go build ./... && go vet ./... && go vet -tags=integration ./... && go vet -tags=e2e ./...`
Expected: no output, exit 0. A compile error here means a patch step was skipped or reordered: use the restart in Global Constraints.

- [ ] **Step 6: Unit tests**

Run: `go test ./... -race -count=1`
Expected: no `FAIL` and no `[FAIL]` anywhere in the output. Raw output shows `ok` per tested package; under the RTK hook it shows one `Go test: <n> passed in <m> packages` line.

- [ ] **Step 7: Integration suite (envtest)**

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race`
Expected: `ok  	github.com/jacaudi/wireguard-operator/internal/controller` after roughly 90 s; no `FAIL`. Known timing flake: if the only failure is the spec `no update churn applies each owned object at most once per reconcile and persists nothing` with the message `a reconcile was still in flight`, rerun the suite once. It failed 1 run in about 15 during this plan's verification, passed 3/3 on rerun of the same commit, and did not reproduce in the plan gate's 23 runs. A second failure, or any other failing spec, is real.

- [ ] **Step 8: Lint**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...`
Expected: the last line is exactly `0 issues.` Gate on that line, never on the exit status (the RTK hook swallows it).

- [ ] **Step 9: Format check (CI's `gofmt / goimports check`)**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint fmt --diff; echo fmt-exit=$?`
Expected: only `fmt-exit=0`. Any diff printed above it fails CI's required `ci` check even when lint says `0 issues.` (the lint ratchet does not see formatting); run `gofmt -w` on the files it names, rerun, and keep the result in this task's commit.

- [ ] **Step 10: YAML lint**

Run: `yamllint .`
Expected: no line containing `error`. One pre-existing `warning` for `./taskfile.yml` 52:3 is normal.

- [ ] **Step 11: Commit**

```bash
git add -A -- cmd internal
git commit -m "refactor(agent)!: delete the iptables sync that flushes the pod's rules" -m "Iptables.Sync ran iptables-restore without --noflush and masqueraded -o eth0. Next to gluetun it would wipe gluetun's rules and leak traffic; the replacement datapath lands in M4." -m "BREAKING CHANGE: the server no longer NATs peer traffic or enforces per-peer egress policy, so peers have no internet egress until M4 routes it through gluetun."
```

- [ ] **Step 12: Confirm the tree is clean and the commit is signed**

Run: `git status --porcelain`
Expected: no output. If anything is listed, it is a path this task touched but the `git add` above missed: `git add` it, `git commit --amend --no-edit`, rerun this step. A second non-empty result is `PLAN: BLOCKED — every path is a guess: dirty tree after commit`.

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. Any other value, a signing error, or a hang at the commit step is `PLAN: BLOCKED` (never `--no-gpg-sign`).

- [ ] **Step 13: Generated files and release.yaml still match their sources**

Run: `make -s manifests generate && git status --porcelain`
Expected: no output. If files are listed, the regenerate block (Task 2 Step 5) was skipped or an API edit came after it: `git add -A -- api config && git commit --amend --no-edit`, rerun. A second failure is `PLAN: BLOCKED — every path is a guess: codegen drift`.

Run: `sh hack/release-file-drift.sh`
Expected: the last line is `release.yaml matches config/`. If not, rerun the regenerate block (Task 2 Step 5 shows it), `git add release.yaml && git commit --amend --no-edit`, rerun this check. A second failure is `PLAN: BLOCKED — every path is a guess: release.yaml drift`.

---

### Task 2: Delete the agent metrics exporter

Kind: refactor

**Files:**
- Create: `internal/resources/deployment_test.go`
- Delete: `internal/agent/wireguard_metrics.go`, `config/prometheus/kustomization.yaml`, `config/prometheus/monitor.yaml`, `config/default/metrics_reader_patch.yaml`, `grafana-dashboard.json`, `readme/screenshot.png`
- Modify: `cmd/agent/main.go`, `api/v1alpha1/wireguard_types.go` (`Metric`), `internal/resources/service.go` (`MetricsPort`, `ForWireguardMetrics`), `internal/resources/deployment.go` (metrics container port), `internal/controller/wireguard_controller.go` (metrics Service apply), `internal/controller/churn_test.go` (the Service becomes the reconcile counter), `internal/controller/apply_test.go`, `internal/it/it_test.go` (the "exposes Prometheus metrics" spec), `config/default/kustomization.yaml`, `images/agent/Dockerfile` (dead `PROMETHEUS_WIREGUARD_EXPORTER_SRC` ARGs), `README.md` (the Grafana screenshot), `go.mod`
- Regenerated: `api/v1alpha1/zz_generated.deepcopy.go`, `config/crd/bases/vpn.wireguard-operator.io_wireguards.yaml`, `release.yaml`

**Interfaces:**
- Consumes: Task 1's `cmd/agent/main.go`.
- Produces: test helpers in package `resources` that Tasks 6 and 7 reuse: `testScheme(t *testing.T) *runtime.Scheme`, `testWireguard() *v1alpha1.Wireguard` (name `vpn`, namespace `default`), and `buildDeployment(t *testing.T, wg *v1alpha1.Wireguard) *appsv1.Deployment`. The patch removes `resources.MetricsPort`, `(*ServiceBuilder).ForWireguardMetrics` and `WireguardSpec.Metric`.

- [ ] **Step 1: Write the failing test**

```bash
awk -v id=t2-deployment-test '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md > internal/resources/deployment_test.go
```

<!-- patch t2-deployment-test -->
~~~~go
package resources

import (
	"maps"
	"testing"

	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	return s
}

func testWireguard() *v1alpha1.Wireguard {
	return &v1alpha1.Wireguard{ObjectMeta: metav1.ObjectMeta{Name: "vpn", Namespace: "default"}}
}

func buildDeployment(t *testing.T, wg *v1alpha1.Wireguard) *appsv1.Deployment {
	t.Helper()
	dep, err := NewDeploymentBuilder(testScheme(t), "agent:test", corev1.PullIfNotPresent).ForWireguard(wg)
	if err != nil {
		t.Fatalf("ForWireguard: %v", err)
	}
	return dep
}

func TestAgentContainerExposesOnlyWireguardAndHealthPorts(t *testing.T) {
	containers := buildDeployment(t, testWireguard()).Spec.Template.Spec.Containers
	if len(containers) != 1 || containers[0].Name != "agent" {
		t.Fatalf("containers = %v, want exactly [agent]", containers)
	}

	got := map[string]int32{}
	for _, p := range containers[0].Ports {
		got[p.Name] = p.ContainerPort
	}
	want := map[string]int32{"wireguard": WireguardPort, "http": HTTPPort}
	if !maps.Equal(got, want) {
		t.Errorf("agent ports = %v, want %v", got, want)
	}
}
~~~~

Run: `wc -l < internal/resources/deployment_test.go | tr -d ' '`
Expected: `50`. Anything else means the extraction picked up the wrong block: delete the file and rerun the step.

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/resources/ -run TestAgentContainerExposesOnlyWireguardAndHealthPorts -count=1`
Expected: FAIL with `agent ports = map[http:8080 metrics:9586 wireguard:51820], want map[http:8080 wireguard:51820]`.

- [ ] **Step 3: Delete the exporter, its Service, its manifests and its dashboard**

```bash
git rm -q internal/agent/wireguard_metrics.go grafana-dashboard.json readme/screenshot.png config/default/metrics_reader_patch.yaml
git rm -rq config/prometheus
```

- [ ] **Step 4: Remove every remaining reference**

```bash
awk -v id=t2-impl '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t2-impl -->
~~~~diff
diff --git a/README.md b/README.md
index 0e33765..8911bda 100644
--- a/README.md
+++ b/README.md
@@ -1,5 +1,4 @@
 # Wireguard Operator
-<img width="1394" alt="Grafana dashboard screenshot" src="readme/screenshot.png">
 
 Painless deployment of wireguard on kubernetes
 
diff --git a/api/v1alpha1/wireguard_types.go b/api/v1alpha1/wireguard_types.go
index 542fe9b..b112432 100644
--- a/api/v1alpha1/wireguard_types.go
+++ b/api/v1alpha1/wireguard_types.go
@@ -77,7 +77,6 @@ type WireguardSpec struct {
 	// A list of Kubernetes taint tolerations applied to the Wireguard pod.
 	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
 	Agent       WireguardPodSpec    `json:"agent,omitempty"`
-	Metric      WireguardPodSpec    `json:"metric,omitempty"`
 	// Tunnel configures optional traffic obfuscation. When enabled, a sidecar
 	// container tunnels WireGuard UDP traffic over WebSocket/TLS.
 	Tunnel TunnelSpec `json:"tunnel,omitempty"`
diff --git a/cmd/agent/main.go b/cmd/agent/main.go
index c753e24..64abca0 100644
--- a/cmd/agent/main.go
+++ b/cmd/agent/main.go
@@ -23,14 +23,12 @@ func main() {
 	var wgUserspaceImplementationFallback string
 	var wireguardListenPort int
 	var wgUseUserspaceImpl bool
-	var metricsBindAddress string
 	flag.StringVar(&configFilePath, "state", "./state.json", "The location of the file that states the desired state")
 	flag.StringVar(&iface, "wg-iface", "wg0", "the wg device name. Default is wg0")
 	flag.StringVar(&wgUserspaceImplementationFallback, "wg-userspace-implementation-fallback", "wireguard-go", "The userspace implementation of wireguard to fallback to")
 	flag.IntVar(&wireguardListenPort, "wg-listen-port", 51820, "the UDP port wireguard is listening on")
 	flag.IntVar(&verbosity, "v", 1, "the verbosity level")
 	flag.BoolVar(&wgUseUserspaceImpl, "wg-use-userspace-implementation", false, "Use userspace implementation")
-	flag.StringVar(&metricsBindAddress, "metrics-bind-address", ":9586", "The address the Prometheus metrics endpoint binds to.")
 	flag.Parse()
 
 	println(fmt.Sprintf(
@@ -83,8 +81,6 @@ func main() {
 
 	close, err := agent.OnStateChange(configFilePath, log.WithName("onStateChange"), func(state agent.State) {
 		log.Info("Received a new state")
-		// Update metrics mapping for peer_name label
-		agent.UpdatePeerNameMapping(state.Peers)
 		err := wg.Sync(state)
 		if err != nil {
 			log.Error(err, "Error while sycncing wireguard")
@@ -130,12 +126,6 @@ func main() {
 		w.WriteHeader(http.StatusOK)
 	})
 
-	// Start metrics endpoint on a separate server
-	go func() { _ = agent.StartMetricsServer(metricsBindAddress, httpLog) }()
-
-	// Register collector
-	agent.RegisterWireguardCollector(iface)
-
 	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
 	defer stop()
 
diff --git a/config/default/kustomization.yaml b/config/default/kustomization.yaml
index bf022b5..45e9ab6 100644
--- a/config/default/kustomization.yaml
+++ b/config/default/kustomization.yaml
@@ -21,8 +21,6 @@ resources:
 #- ../webhook
 # [CERTMANAGER] To enable cert-manager, uncomment all sections with 'CERTMANAGER'. 'WEBHOOK' components are required.
 #- ../certmanager
-# [PROMETHEUS] To enable prometheus monitor, uncomment all sections with 'PROMETHEUS'.
-#- ../prometheus
 
 # Mount the controller config file for loading manager configurations
 # through a ComponentConfig type
@@ -42,11 +40,3 @@ apiVersion: kustomize.config.k8s.io/v1beta1
 kind: Kustomization
 patches:
 - path: manager_args_patch.yaml
-# [PROMETHEUS] If you enabled prometheus above, uncomment this to update ClusterRoleBinding with correct prometheus SA.
-# update the patch to change the name or namespace for the prometheus SA
-#- path: metrics_reader_patch.yaml
-#  target:
-#    group: rbac.authorization.k8s.io
-#    version: v1
-#    kind: ClusterRoleBinding
-#    name: metrics-reader
diff --git a/go.mod b/go.mod
index f853c89..c04ea3d 100644
--- a/go.mod
+++ b/go.mod
@@ -20,7 +20,6 @@ require (
 	github.com/korylprince/ipnetgen v1.0.1
 	github.com/onsi/ginkgo v1.16.5
 	github.com/onsi/gomega v1.33.1
-	github.com/prometheus/client_golang v1.16.0
 	github.com/vishvananda/netlink v1.1.0
 	golang.org/x/sys v0.48.0
 	golang.zx2c4.com/wireguard/wgctrl v0.0.0-20230429144221-925a1e7659e6
@@ -75,6 +74,7 @@ require (
 	github.com/onsi/ginkgo/v2 v2.19.0 // indirect
 	github.com/pelletier/go-toml v1.9.5 // indirect
 	github.com/pkg/errors v0.9.1 // indirect
+	github.com/prometheus/client_golang v1.16.0 // indirect
 	github.com/prometheus/client_model v0.4.0 // indirect
 	github.com/prometheus/common v0.44.0 // indirect
 	github.com/prometheus/procfs v0.12.0 // indirect
diff --git a/images/agent/Dockerfile b/images/agent/Dockerfile
index 79bb4e5..f714796 100644
--- a/images/agent/Dockerfile
+++ b/images/agent/Dockerfile
@@ -1,6 +1,5 @@
 ARG WIREGUARD_GO_SRC=/usr/local/src/wireguard-go
 ARG WIREGUARD_AGENT_SRC=/usr/local/src/wireguard-agent
-ARG PROMETHEUS_WIREGUARD_EXPORTER_SRC=/usr/local/src/prometheus-wireguard-exporter
 
 # step 1: Build Agent
 FROM --platform=${BUILDPLATFORM} golang:1.26 AS golang-builder
@@ -35,7 +34,6 @@ RUN set -eux; \
 FROM debian:trixie
 ARG WIREGUARD_GO_SRC
 ARG WIREGUARD_AGENT_SRC
-ARG PROMETHEUS_WIREGUARD_EXPORTER_SRC
 
 RUN apt-get update \
     && apt-get install --no-install-recommends -y iptables wireguard-tools \
diff --git a/internal/controller/apply_test.go b/internal/controller/apply_test.go
index c5883f0..391595a 100644
--- a/internal/controller/apply_test.go
+++ b/internal/controller/apply_test.go
@@ -204,10 +204,8 @@ var _ = Describe("applyOwned", func() {
 			_ = k8sClient.Delete(context.Background(), wg)
 			_ = k8sClient.Delete(context.Background(), &appsv1.Deployment{
 				ObjectMeta: metav1.ObjectMeta{Name: nn.Name + "-dep", Namespace: applyNS}})
-			for _, svcName := range []string{nn.Name + "-svc", nn.Name + "-metrics-svc"} {
-				_ = k8sClient.Delete(context.Background(), &corev1.Service{
-					ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: applyNS}})
-			}
+			_ = k8sClient.Delete(context.Background(), &corev1.Service{
+				ObjectMeta: metav1.ObjectMeta{Name: nn.Name + "-svc", Namespace: applyNS}})
 		}()
 
 		_, err := r.failDegraded(context.Background(),
diff --git a/internal/controller/churn_test.go b/internal/controller/churn_test.go
index 3bb33b8..c985369 100644
--- a/internal/controller/churn_test.go
+++ b/internal/controller/churn_test.go
@@ -131,19 +131,19 @@ var _ = Describe("no update churn", func() {
 		}, churnTimeout, churnInterval).Should(Succeed())
 
 		wgKey := types.NamespacedName{Name: churnName, Namespace: churnNS}
-		metricsSvc := churnName + "-metrics-svc"
+		svcName := churnName + "-svc"
 
-		// awaitQuiet returns once no reconcile is in flight: the metrics Service is the first object a
-		// reconcile applies, so two seconds without a metrics write means the cascade has finished.
+		// awaitQuiet returns once no reconcile is in flight: the Service is the first object a
+		// reconcile applies, so two seconds without a Service write means the cascade has finished.
 		awaitQuiet := func() {
-			n := writeCounter.writesTo(churnNS, metricsSvc)
-			Consistently(func() int { return writeCounter.writesTo(churnNS, metricsSvc) },
+			n := writeCounter.writesTo(churnNS, svcName)
+			Consistently(func() int { return writeCounter.writesTo(churnNS, svcName) },
 				"2s", churnInterval).Should(Equal(n), "a reconcile was still in flight")
 		}
 
 		// Let the create cascade finish. Ready=True is written only by a reconcile that applied all
-		// three objects, and the status write it causes triggers one more reconcile; wait that out
-		// too, so reset() cannot land between a reconcile's metrics apply and its Deployment apply.
+		// two objects, and the status write it causes triggers one more reconcile; wait that out
+		// too, so reset() cannot land between a reconcile's Service apply and its Deployment apply.
 		Eventually(func() bool {
 			live := &v1alpha1.Wireguard{}
 			if err := k8sClient.Get(context.Background(), wgKey, live); err != nil {
@@ -168,9 +168,9 @@ var _ = Describe("no update churn", func() {
 		//
 		// This is NOT a no-op for every owned object: agent.State embeds the whole Wireguard
 		// (wireguard_controller.go, `Server: *wireguard.DeepCopy()`), so the annotation is
-		// serialised into state.json and the peer-config Secret is legitimately rewritten. That
-		// is why the assertions below name the Deployment and Services explicitly rather than
-		// counting everything in the namespace.
+		// serialised into state.json and the server Secret is legitimately rewritten. That is why
+		// the assertions below name the Deployment and Service explicitly rather than counting
+		// everything in the namespace.
 		for i := range 3 {
 			Eventually(func() error {
 				live := &v1alpha1.Wireguard{}
@@ -196,23 +196,20 @@ var _ = Describe("no update churn", func() {
 		}, "3s", churnInterval).Should(Equal(settled.ResourceVersion),
 			"the Deployment must not be rewritten when nothing it renders has changed")
 
-		// The metrics Service is applied exactly once per reconcile: one call site, no branches.
-		// It is therefore a reconcile counter. The Deployment must match it one-for-one -- the
+		// The Service is applied exactly once per reconcile: one call site, no branches. It is
+		// therefore a reconcile counter. The Deployment must match it one-for-one -- the
 		// pre-change code could write the Deployment up to four times in a single reconcile.
 		awaitQuiet()
-		metricsWrites := writeCounter.writesTo(churnNS, churnName+"-metrics-svc")
+		svcWrites := writeCounter.writesTo(churnNS, svcName)
 		depWrites := writeCounter.writesTo(churnNS, churnName+"-dep")
-		svcWrites := writeCounter.writesTo(churnNS, churnName+"-svc")
 
-		Expect(metricsWrites).Should(BeNumerically(">", 0), "sanity: the reconciler should have run")
-		Expect(depWrites).Should(Equal(metricsWrites),
+		Expect(svcWrites).Should(BeNumerically(">", 0), "sanity: the reconciler should have run")
+		Expect(depWrites).Should(Equal(svcWrites),
 			"the Deployment must be applied exactly once per reconcile")
-		Expect(svcWrites).Should(Equal(metricsWrites),
-			"the Service must be applied exactly once per reconcile")
 
 		// SSA-born objects carry no legacy entry, so applyOwned must never send them a migration
 		// patch -- counted from creation, not from reset().
-		Expect(writeCounter.migrationsTo(churnNS, churnName+"-dep", churnName+"-svc", churnName+"-metrics-svc")).
+		Expect(writeCounter.migrationsTo(churnNS, churnName+"-dep", svcName)).
 			Should(BeZero(), "an object the operator created by apply must never be migrated")
 	})
 })
diff --git a/internal/controller/wireguard_controller.go b/internal/controller/wireguard_controller.go
index 9c09a75..c30392e 100644
--- a/internal/controller/wireguard_controller.go
+++ b/internal/controller/wireguard_controller.go
@@ -498,15 +498,6 @@ func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (
 		filteredPeers = append(filteredPeers, peer)
 	}
 
-	// metrics service
-	desiredMetricsSvc, err := r.serviceBuilder.ForWireguardMetrics(wireguard)
-	if err != nil {
-		return r.failDegraded(ctx, req, err, "MetricsServiceBuildFailed", "Failed to build metrics service")
-	}
-	if err := r.applyOwned(ctx, desiredMetricsSvc); err != nil {
-		return r.failDegraded(ctx, req, err, "MetricsServiceApplyFailed", "Failed to apply metrics service")
-	}
-
 	serviceType := corev1.ServiceTypeLoadBalancer
 
 	if wireguard.Spec.ServiceType != "" {
diff --git a/internal/it/it_test.go b/internal/it/it_test.go
index d6af94a..bd716fe 100644
--- a/internal/it/it_test.go
+++ b/internal/it/it_test.go
@@ -46,37 +46,6 @@ spec:
 		// TODO: connect to wg
 	})
 
-	It("exposes Prometheus metrics", func() {
-		// Ensure the Wireguard server exists and is ready from previous spec
-		WaitForWireguardToBeReady("vpn", TestNamespace)
-
-		metricsCheckPod := `
-apiVersion: v1
-kind: Pod
-metadata:
-  name: metrics-check
-spec:
-  restartPolicy: Never
-  containers:
-  - name: curl
-    image: curlimages/curl:8.17.0
-    command: ["sh","-c"]
-    args:
-    - >
-      curl -sS http://vpn-metrics-svc.default.svc.cluster.local:9586/metrics |
-      grep -E 'wireguard_(sent_bytes_total|received_bytes_total|latest_handshake_seconds)'
-`
-		output, err := KubectlApply(metricsCheckPod, TestNamespace)
-		Expect(err).NotTo(HaveOccurred())
-		Expect(output).To(Equal("pod/metrics-check created"))
-
-		WaitForPodSucceeded("metrics-check", TestNamespace)
-
-		logs, err := KubectlLogs("metrics-check", TestNamespace)
-		Expect(err).NotTo(HaveOccurred())
-		Expect(logs).To(ContainSubstring("wireguard_"))
-	})
-
 	It("enables wstunnel sidecar when tunnel is configured", func() {
 		// Ensure the Wireguard server exists and is ready from previous spec
 		WaitForWireguardToBeReady("vpn", TestNamespace)
diff --git a/internal/resources/deployment.go b/internal/resources/deployment.go
index c319bf5..0907e29 100644
--- a/internal/resources/deployment.go
+++ b/internal/resources/deployment.go
@@ -168,11 +168,6 @@ func (b *DeploymentBuilder) agentContainer(wg *v1alpha1.Wireguard, readOnlyRootF
 				Name:          "http",
 				Protocol:      corev1.ProtocolTCP,
 			},
-			{
-				ContainerPort: MetricsPort,
-				Name:          "metrics",
-				Protocol:      corev1.ProtocolTCP,
-			},
 		},
 		EnvFrom: []corev1.EnvFromSource{
 			{
diff --git a/internal/resources/service.go b/internal/resources/service.go
index 62ddad3..2e19fe7 100644
--- a/internal/resources/service.go
+++ b/internal/resources/service.go
@@ -27,8 +27,6 @@ import (
 const (
 	// WireguardPort is the default port for Wireguard VPN.
 	WireguardPort = 51820
-	// MetricsPort is the port for exposing metrics.
-	MetricsPort = 9586
 	// DefaultTunnelPort is the default port for the wstunnel sidecar.
 	DefaultTunnelPort = 443
 )
@@ -99,32 +97,3 @@ func (b *ServiceBuilder) ForWireguard(wg *v1alpha1.Wireguard, serviceType corev1
 
 	return svc, nil
 }
-
-// ForWireguardMetrics creates a metrics service for a Wireguard server.
-func (b *ServiceBuilder) ForWireguardMetrics(wg *v1alpha1.Wireguard) (*corev1.Service, error) {
-	labels := LabelsForWireguard(wg.Name)
-
-	svc := &corev1.Service{
-		ObjectMeta: metav1.ObjectMeta{
-			Name:      wg.Name + "-metrics-svc",
-			Namespace: wg.Namespace,
-			Labels:    labels,
-		},
-		Spec: corev1.ServiceSpec{
-			Selector: labels,
-			Ports: []corev1.ServicePort{{
-				Name:       "metrics",
-				Protocol:   corev1.ProtocolTCP,
-				Port:       MetricsPort,
-				TargetPort: intstr.FromInt(MetricsPort),
-			}},
-			Type: corev1.ServiceTypeClusterIP,
-		},
-	}
-
-	if err := SetOwnerReference(wg, svc, b.scheme); err != nil {
-		return nil, err
-	}
-
-	return svc, nil
-}
~~~~

- [ ] **Step 5: Regenerate CRDs, deepcopy and release.yaml**

```bash
make manifests generate &&
AGENT=$(sed -n 's/^[[:space:]]*- --agent-image=//p' release.yaml | head -1) &&
MGR=$(sed -n 's/^        image: //p' release.yaml | head -1) &&
make generate-release-file AGENT_IMAGE="$AGENT" MANAGER_IMAGE="$MGR" > /dev/null &&
echo regen-ok
```

Expected: the block's last line is `regen-ok`. Anything else (a controller-gen or kustomize error) is `PLAN: BLOCKED — every path is a guess: regenerate`.

- [ ] **Step 6: Module graph is tidy**

Run: `go mod tidy && git status --porcelain -- go.sum; grep -n 'prometheus/client_golang' go.mod`
Expected: no `go.sum` line, then exactly one `go.mod` line, ending `github.com/prometheus/client_golang v1.16.0 // indirect`.

- [ ] **Step 7: The new test passes**

Run: `go test ./internal/resources/ -run TestAgentContainerExposesOnlyWireguardAndHealthPorts -count=1`
Expected: PASS.

- [ ] **Step 8: Build and vet every build tag**

Run: `go build ./... && go vet ./... && go vet -tags=integration ./... && go vet -tags=e2e ./...`
Expected: no output, exit 0. A compile error here means a patch step was skipped or reordered: use the restart in Global Constraints.

- [ ] **Step 9: Unit tests**

Run: `go test ./... -race -count=1`
Expected: no `FAIL` and no `[FAIL]` anywhere in the output. Raw output shows `ok` per tested package; under the RTK hook it shows one `Go test: <n> passed in <m> packages` line.

- [ ] **Step 10: Integration suite (envtest)**

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race`
Expected: `ok  	github.com/jacaudi/wireguard-operator/internal/controller` after roughly 90 s; no `FAIL`. Known timing flake: if the only failure is the spec `no update churn applies each owned object at most once per reconcile and persists nothing` with the message `a reconcile was still in flight`, rerun the suite once. It failed 1 run in about 15 during this plan's verification, passed 3/3 on rerun of the same commit, and did not reproduce in the plan gate's 23 runs. A second failure, or any other failing spec, is real.

- [ ] **Step 11: Lint**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...`
Expected: the last line is exactly `0 issues.` Gate on that line, never on the exit status (the RTK hook swallows it).

- [ ] **Step 12: Format check (CI's `gofmt / goimports check`)**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint fmt --diff; echo fmt-exit=$?`
Expected: only `fmt-exit=0`. Any diff printed above it fails CI's required `ci` check even when lint says `0 issues.` (the lint ratchet does not see formatting); run `gofmt -w` on the files it names, rerun, and keep the result in this task's commit.

- [ ] **Step 13: YAML lint**

Run: `yamllint .`
Expected: no line containing `error`. One pre-existing `warning` for `./taskfile.yml` 52:3 is normal.

- [ ] **Step 14: Commit**

```bash
git add -A -- api cmd config internal images README.md go.mod release.yaml
git commit -m "refactor!: delete the agent metrics exporter" -m "The wireguard collector, its metrics Service and port, spec.metric, config/prometheus and the Grafana dashboard have no present consumer (design 2.2). The churn spec now counts reconciles by Service writes." -m "BREAKING CHANGE: spec.metric, the \`<name>-metrics-svc\` Service and the agent's :9586 metrics port are removed."
```

- [ ] **Step 15: Confirm the tree is clean and the commit is signed**

Run: `git status --porcelain`
Expected: no output. If anything is listed, it is a path this task touched but the `git add` above missed: `git add` it, `git commit --amend --no-edit`, rerun this step. A second non-empty result is `PLAN: BLOCKED — every path is a guess: dirty tree after commit`.

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. Any other value, a signing error, or a hang at the commit step is `PLAN: BLOCKED` (never `--no-gpg-sign`).

- [ ] **Step 16: Generated files and release.yaml still match their sources**

Run: `make -s manifests generate && git status --porcelain`
Expected: no output. If files are listed, the regenerate block (Task 2 Step 5) was skipped or an API edit came after it: `git add -A -- api config && git commit --amend --no-edit`, rerun. A second failure is `PLAN: BLOCKED — every path is a guess: codegen drift`.

Run: `sh hack/release-file-drift.sh`
Expected: the last line is `release.yaml matches config/`. If not, rerun the regenerate block (Task 2 Step 5 shows it), `git add release.yaml && git commit --amend --no-edit`, rerun this check. A second failure is `PLAN: BLOCKED — every path is a guess: release.yaml drift`.

---

### Task 3: Delete the wstunnel sidecar

Kind: refactor

**Files:**
- Modify: `api/v1alpha1/wireguard_types.go` (`TunnelSpec`, `Spec.Tunnel`, `Status.Tunnel`, the `Tunnel` print column), `internal/resources/deployment.go` (`DefaultWstunnelImage`, `wstunnelContainer`), `internal/resources/service.go` (`DefaultTunnelPort`, tunnel port), `internal/controller/wireguard_controller.go` (tunnel peer-config branch, tunnel port, tunnel status), `internal/controller/wireguard_controller_test.go`, `internal/it/it_test.go`
- Regenerated: deepcopy, `vpn.wireguard-operator.io_wireguards.yaml`, `release.yaml`

**Interfaces:**
- Consumes: Task 2's tree.
- Produces: no `Tunnel*` symbol remains anywhere. The direct `[Peer]` rendering in `updateWireguardPeers` is unchanged here (now `pureCfg +=`), and Task 5 deletes it.

wstunnel is opt-in and off by default, so removing it changes nothing a default object does: refactor rule, no new failing test. Its specs go with it; the SSA-adoption spec switches its legacy-owned field from the sidecar to a toleration.

- [ ] **Step 1: Remove the wstunnel specs (whole Ginkgo blocks, anchored by title)**

Each `perl` range deletes from the exact `It(`/`Context(` line through the first closing `})` at the same indent.

```bash
perl -i -ne 'print unless /^\tContext\("Tunnel", func\(\) \{$/ .. /^\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t\tIt\("renders keepalive in the tunnel and dual-mode direct configs", func\(\) \{$/ .. /^\t\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\tIt\("enables wstunnel sidecar when tunnel is configured", func\(\) \{$/ .. /^\t\}\)$/' internal/it/it_test.go
perl -i -ne 'print unless /^\tIt\("connects to WireGuard server through wstunnel tunnel", func\(\) \{$/ .. /^\t\}\)$/' internal/it/it_test.go
gofmt -w internal/controller/wireguard_controller_test.go internal/it/it_test.go
```

Run: `cat internal/controller/wireguard_controller_test.go internal/it/it_test.go | grep -cF -e 'Context("Tunnel"' -e 'renders keepalive in the tunnel and dual-mode direct configs' -e 'enables wstunnel sidecar when tunnel is configured' -e 'connects to WireGuard server through wstunnel tunnel'`
Expected: `0`. A non-zero count means an anchor did not match (the file is not in the state this plan assumes): use the restart in Global Constraints; if it recurs, `PLAN: BLOCKED — every path is a guess: plan drift <task>/<step>`.

Run: `grep -ci 'tunnel' internal/it/it_test.go; grep -c 'It(' internal/it/it_test.go`
Expected: `0` then `2` ("wireguard is able to start" and "exposes controller manager metrics" remain).

- [ ] **Step 2: Adapt the remaining specs**

```bash
awk -v id=t3-test '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t3-test -->
~~~~diff
diff --git a/internal/controller/wireguard_controller_test.go b/internal/controller/wireguard_controller_test.go
index 7b4a447..13d56f5 100644
--- a/internal/controller/wireguard_controller_test.go
+++ b/internal/controller/wireguard_controller_test.go
@@ -1694,14 +1694,18 @@ var _ = Describe("wireguard controller", func() {
 
 			// Seed the legacy object BEFORE the Wireguard exists -- otherwise the operator creates
 			// the Deployment by apply first and this spec tests nothing. The real builder renders
-			// exactly what the pre-change operator wrote, wstunnel sidecar included; its owner
+			// exactly what the pre-change operator wrote, a toleration included; its owner
 			// reference is dropped because the Wireguard has no UID yet (the operator's apply adds
 			// one). "manager" is the field manager the production binary recorded before this change.
 			// One of the two legacy seeds in the suite (see the Service variant below); design §7.3.
 			owner := &v1alpha1.Wireguard{
 				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
 			}
-			owner.Spec.Tunnel.Enabled = true
+			owner.Spec.Tolerations = []corev1.Toleration{{
+				Key:      "probe.test/legacy",
+				Operator: corev1.TolerationOpExists,
+				Effect:   corev1.TaintEffectNoSchedule,
+			}}
 			legacy, err := resources.NewDeploymentBuilder(k8sClient.Scheme(), wgTestImage, corev1.PullIfNotPresent).
 				ForWireguard(owner)
 			Expect(err).ShouldNot(HaveOccurred())
@@ -1742,23 +1746,19 @@ var _ = Describe("wireguard controller", func() {
 				if err := k8sClient.Get(context.Background(), wgKey, wg); err != nil {
 					return err
 				}
-				wg.Spec.Tunnel.Enabled = false
+				wg.Spec.Tolerations = nil
 				return k8sClient.Update(context.Background(), wg)
 			}, Timeout, Interval).Should(Succeed())
 
 			// The proof that adoption is complete. Without the legacy-manager migration in
-			// applyOwned, "manager" still co-owns the sidecar and the apiserver never prunes it.
-			Eventually(func() []string {
+			// applyOwned, "manager" still co-owns the toleration and the apiserver never prunes it.
+			Eventually(func() []corev1.Toleration {
 				dep := &appsv1.Deployment{}
 				if err := k8sClient.Get(context.Background(), deploymentKey, dep); err != nil {
-					return nil
+					return []corev1.Toleration{{Key: "get-failed"}}
 				}
-				var names []string
-				for _, c := range dep.Spec.Template.Spec.Containers {
-					names = append(names, c.Name)
-				}
-				return names
-			}, Timeout, Interval).Should(Equal([]string{"agent"}), "the legacy-owned wstunnel sidecar must be removed")
+				return dep.Spec.Template.Spec.Tolerations
+			}, Timeout, Interval).Should(BeEmpty(), "the legacy-owned toleration must be removed")
 
 			removed := &appsv1.Deployment{}
 			Expect(k8sClient.Get(context.Background(), deploymentKey, removed)).Should(Succeed())
diff --git a/internal/it/it_test.go b/internal/it/it_test.go
index 68be7b8..810146a 100644
--- a/internal/it/it_test.go
+++ b/internal/it/it_test.go
@@ -4,8 +4,6 @@
 package it
 
 import (
-	"fmt"
-
 	. "github.com/onsi/ginkgo"
 	. "github.com/onsi/gomega"
 )
~~~~

- [ ] **Step 3: Delete wstunnel from the API, builders and controller**

```bash
awk -v id=t3-impl '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t3-impl -->
~~~~diff
diff --git a/api/v1alpha1/wireguard_types.go b/api/v1alpha1/wireguard_types.go
index b112432..2741574 100644
--- a/api/v1alpha1/wireguard_types.go
+++ b/api/v1alpha1/wireguard_types.go
@@ -77,30 +77,6 @@ type WireguardSpec struct {
 	// A list of Kubernetes taint tolerations applied to the Wireguard pod.
 	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
 	Agent       WireguardPodSpec    `json:"agent,omitempty"`
-	// Tunnel configures optional traffic obfuscation. When enabled, a sidecar
-	// container tunnels WireGuard UDP traffic over WebSocket/TLS.
-	Tunnel TunnelSpec `json:"tunnel,omitempty"`
-}
-
-// TunnelSpec configures traffic obfuscation via a tunneling sidecar.
-type TunnelSpec struct {
-	// Enabled activates the tunnel sidecar.
-	Enabled bool `json:"enabled,omitempty"`
-	// Type selects the tunneling implementation. Currently only "wstunnel" is supported.
-	// +kubebuilder:validation:Enum=wstunnel
-	// +kubebuilder:default=wstunnel
-	Type string `json:"type,omitempty"`
-	// Port is the external port the tunnel listens on. Default: 443.
-	// +kubebuilder:default=443
-	Port int32 `json:"port,omitempty"`
-	// Image is the container image for the tunnel sidecar. Default: "ghcr.io/erebe/wstunnel:latest".
-	Image string `json:"image,omitempty"`
-	// DualMode exposes both the direct WireGuard UDP port and the tunnel port on the service,
-	// and generates both a direct and a tunnel peer config. When false (default), only the
-	// tunnel port is exposed.
-	DualMode bool `json:"dualMode,omitempty"`
-	// Resources for the tunnel sidecar container.
-	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
 }
 
 // WireguardPodSpec defines spec for respective containers created for Wireguard
@@ -119,8 +95,6 @@ type WireguardStatus struct {
 	Status string `json:"status,omitempty"`
 	// A string field that provides additional information about the status of Wireguard. This could include error messages or other information that helps to diagnose issues with the wg instance.
 	Message string `json:"message,omitempty"`
-	// The active tunnel type (e.g., "wstunnel") or empty when tunneling is disabled.
-	Tunnel string `json:"tunnel,omitempty"`
 	// A stable identifier for the Wireguard instance (e.g., server public key).
 	UniqueIdentifier string `json:"uniqueIdentifier,omitempty"`
 	// Resource-level status information for associated Kubernetes resources.
@@ -150,7 +124,6 @@ type Resource struct {
 //+kubebuilder:subresource:status
 //+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
 //+kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.address`
-//+kubebuilder:printcolumn:name="Tunnel",type=string,JSONPath=`.status.tunnel`
 //+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
 
 // Wireguard is the Schema for the wireguards API
diff --git a/internal/controller/wireguard_controller.go b/internal/controller/wireguard_controller.go
index c30392e..f284cf6 100644
--- a/internal/controller/wireguard_controller.go
+++ b/internal/controller/wireguard_controller.go
@@ -336,50 +336,14 @@ DNS = %s`, strings.TrimSpace(string(v)), addressLine, dnsConfiguration)
 				if serverMtu != "" {
 					pureCfg = pureCfg + "\nMTU = " + serverMtu
 				}
-				if wireguard.Spec.Tunnel.Enabled {
-					tunnelPort := wireguard.Spec.Tunnel.Port
-					if tunnelPort == 0 {
-						tunnelPort = resources.DefaultTunnelPort
-					}
-
-					// Tunnel config with PreUp/PostDown hooks
-					tunnelCfg := pureCfg + fmt.Sprintf(`
-PreUp = wstunnel client -L udp://127.0.0.1:%d:127.0.0.1:%d wss://%s:%d &
-PostDown = killall wstunnel || true
-
-[Peer]
-PublicKey = %s
-AllowedIPs = %s
-Endpoint = 127.0.0.1:%d
-%s`, resources.WireguardPort, resources.WireguardPort, serverAddress, tunnelPort, serverPublicKey, allowIps, resources.WireguardPort, keepaliveLine)
-
-					if wireguard.Spec.Tunnel.DualMode {
-						// In dual mode, store both configs:
-						// <peer> = direct config (connects via WireGuard UDP)
-						// <peer>.tunnel = tunnel config (connects via wstunnel)
-						directCfg := pureCfg + fmt.Sprintf(`
-
-[Peer]
-PublicKey = %s
-AllowedIPs = %s
-Endpoint = %s:%s
-%s`, serverPublicKey, allowIps, serverAddress, wireguard.Status.Port, keepaliveLine)
-						newPeerCfgData[peer.Name] = []byte(directCfg)
-						newPeerCfgData[peer.Name+".tunnel"] = []byte(tunnelCfg)
-					} else {
-						// Tunnel-only: the main config uses the tunnel
-						newPeerCfgData[peer.Name] = []byte(tunnelCfg)
-					}
-				} else {
-					pureCfg = pureCfg + fmt.Sprintf(`
+				pureCfg += fmt.Sprintf(`
 
 [Peer]
 PublicKey = %s
 AllowedIPs = %s
 Endpoint = %s:%s
 %s`, serverPublicKey, allowIps, serverAddress, wireguard.Status.Port, keepaliveLine)
-					newPeerCfgData[peer.Name] = []byte(pureCfg)
-				}
+				newPeerCfgData[peer.Name] = []byte(pureCfg)
 			}
 		}
 		if peer.Status.Status != v1alpha1.Ready {
@@ -552,15 +516,7 @@ func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (
 		address = wireguard.Spec.Address
 	}
 
-	// Compute the effective external port (tunnel port when enabled, WG port otherwise)
 	var port = fmt.Sprintf("%d", resources.WireguardPort)
-	if wireguard.Spec.Tunnel.Enabled {
-		tp := wireguard.Spec.Tunnel.Port
-		if tp == 0 {
-			tp = resources.DefaultTunnelPort
-		}
-		port = fmt.Sprintf("%d", tp)
-	}
 
 	if serviceType == corev1.ServiceTypeLoadBalancer {
 		ingressList := svcFound.Status.LoadBalancer.Ingress
@@ -835,20 +791,8 @@ func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (
 		}
 		resourcesStatus = append(resourcesStatus, v1alpha1.Resource{Name: desiredDep.Name, Type: "Deployment", Status: depStatus, Image: depImage})
 
-		// Compute tunnel status
-		tunnelStatus := "disabled"
-		if wireguard.Spec.Tunnel.Enabled {
-			tunnelStatus = wireguard.Spec.Tunnel.Type
-			if tunnelStatus == "" {
-				tunnelStatus = "wstunnel"
-			}
-		}
-
 		// Update status if changed
 		needUpdate := false
-		if wireguard.Status.Tunnel != tunnelStatus {
-			needUpdate = true
-		}
 		if wireguard.Status.UniqueIdentifier != uniqueIdentifier {
 			needUpdate = true
 		}
@@ -865,7 +809,6 @@ func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (
 		if needUpdate {
 			latest := &v1alpha1.Wireguard{}
 			if err := r.Get(ctx, req.NamespacedName, latest); err == nil {
-				latest.Status.Tunnel = tunnelStatus
 				latest.Status.UniqueIdentifier = uniqueIdentifier
 				latest.Status.Resources = resourcesStatus
 				if err := r.Status().Update(ctx, latest); err != nil {
diff --git a/internal/resources/deployment.go b/internal/resources/deployment.go
index 0907e29..5bfd2f6 100644
--- a/internal/resources/deployment.go
+++ b/internal/resources/deployment.go
@@ -30,8 +30,6 @@ import (
 const (
 	// HTTPPort is the port for HTTP health checks.
 	HTTPPort = 8080
-	// DefaultWstunnelImage is the default container image for the wstunnel sidecar.
-	DefaultWstunnelImage = "ghcr.io/erebe/wstunnel:latest"
 )
 
 // DeploymentBuilder builds deployments for wireguard resources.
@@ -114,14 +112,6 @@ func (b *DeploymentBuilder) ForWireguard(wg *v1alpha1.Wireguard) (*appsv1.Deploy
 		dep.Spec.Template.Spec.InitContainers = append(dep.Spec.Template.Spec.InitContainers, b.ipForwardInitContainer())
 	}
 
-	// Add wstunnel sidecar if tunnel is enabled
-	if wg.Spec.Tunnel.Enabled {
-		dep.Spec.Template.Spec.Containers = append(
-			dep.Spec.Template.Spec.Containers,
-			b.wstunnelContainer(wg, readOnlyRootFilesystem, allowPrivilegeEscalation),
-		)
-	}
-
 	// Add userspace implementation flag if requested
 	if wg.Spec.UseWgUserspaceImplementation {
 		for i, c := range dep.Spec.Template.Spec.Containers {
@@ -206,47 +196,6 @@ func (b *DeploymentBuilder) agentContainer(wg *v1alpha1.Wireguard, readOnlyRootF
 	}
 }
 
-// wstunnelContainer creates the wstunnel sidecar container for traffic obfuscation.
-func (b *DeploymentBuilder) wstunnelContainer(wg *v1alpha1.Wireguard, readOnlyRootFilesystem, allowPrivilegeEscalation bool) corev1.Container {
-	image := wg.Spec.Tunnel.Image
-	if image == "" {
-		image = DefaultWstunnelImage
-	}
-	tunnelPort := wg.Spec.Tunnel.Port
-	if tunnelPort == 0 {
-		tunnelPort = DefaultTunnelPort
-	}
-	return corev1.Container{
-		SecurityContext: &corev1.SecurityContext{
-			ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
-			AllowPrivilegeEscalation: &allowPrivilegeEscalation,
-		},
-		Image: image,
-		Name:  "wstunnel",
-		Command: []string{
-			"/usr/bin/dumb-init", "--",
-			"/home/app/wstunnel", "server",
-			"--restrict-to", fmt.Sprintf("127.0.0.1:%d", WireguardPort),
-			fmt.Sprintf("wss://0.0.0.0:%d", tunnelPort),
-		},
-		Ports: []corev1.ContainerPort{
-			{
-				ContainerPort: tunnelPort,
-				Name:          "tunnel",
-				Protocol:      corev1.ProtocolTCP,
-			},
-		},
-		ReadinessProbe: &corev1.Probe{
-			ProbeHandler: corev1.ProbeHandler{
-				TCPSocket: &corev1.TCPSocketAction{
-					Port: intstr.FromInt(int(tunnelPort)),
-				},
-			},
-		},
-		Resources: wg.Spec.Tunnel.Resources,
-	}
-}
-
 // ipForwardInitContainer creates an init container to enable IP forwarding.
 func (b *DeploymentBuilder) ipForwardInitContainer() corev1.Container {
 	privileged := true
diff --git a/internal/resources/service.go b/internal/resources/service.go
index 2e19fe7..ede8010 100644
--- a/internal/resources/service.go
+++ b/internal/resources/service.go
@@ -27,8 +27,6 @@ import (
 const (
 	// WireguardPort is the default port for Wireguard VPN.
 	WireguardPort = 51820
-	// DefaultTunnelPort is the default port for the wstunnel sidecar.
-	DefaultTunnelPort = 443
 )
 
 // ServiceBuilder builds services for wireguard resources.
@@ -53,24 +51,6 @@ func (b *ServiceBuilder) ForWireguard(wg *v1alpha1.Wireguard, serviceType corev1
 		TargetPort: intstr.FromInt(WireguardPort),
 	}}
 
-	if wg.Spec.Tunnel.Enabled {
-		tunnelPort := wg.Spec.Tunnel.Port
-		if tunnelPort == 0 {
-			tunnelPort = DefaultTunnelPort
-		}
-		tunnelSvcPort := corev1.ServicePort{
-			Name:       "tunnel",
-			Protocol:   corev1.ProtocolTCP,
-			Port:       tunnelPort,
-			TargetPort: intstr.FromInt(int(tunnelPort)),
-		}
-		if wg.Spec.Tunnel.DualMode {
-			svcPorts = append(svcPorts, tunnelSvcPort)
-		} else {
-			svcPorts = []corev1.ServicePort{tunnelSvcPort}
-		}
-	}
-
 	svc := &corev1.Service{
 		ObjectMeta: metav1.ObjectMeta{
 			Name:        wg.Name + "-svc",
~~~~

- [ ] **Step 4: Regenerate**

```bash
make manifests generate &&
AGENT=$(sed -n 's/^[[:space:]]*- --agent-image=//p' release.yaml | head -1) &&
MGR=$(sed -n 's/^        image: //p' release.yaml | head -1) &&
make generate-release-file AGENT_IMAGE="$AGENT" MANAGER_IMAGE="$MGR" > /dev/null &&
echo regen-ok
```

Expected: the block's last line is `regen-ok`. Anything else (a controller-gen or kustomize error) is `PLAN: BLOCKED — every path is a guess: regenerate`.

- [ ] **Step 5: No wstunnel reference survives outside history files**

Run: `grep -rni 'wstunnel\|TunnelSpec\|DefaultTunnelPort' --include='*.go' --include='*.yaml' --exclude=apply.go api cmd config internal examples release.yaml ; echo grep-exit=$?`
Expected: only `grep-exit=1`. (`internal/controller/apply.go` is excluded: its doc comment names the wstunnel sidecar as a historical example, and the design keeps that file as-is.)

- [ ] **Step 6: Build and vet every build tag**

Run: `go build ./... && go vet ./... && go vet -tags=integration ./... && go vet -tags=e2e ./...`
Expected: no output, exit 0. A compile error here means a patch step was skipped or reordered: use the restart in Global Constraints.

- [ ] **Step 7: Unit tests**

Run: `go test ./... -race -count=1`
Expected: no `FAIL` and no `[FAIL]` anywhere in the output. Raw output shows `ok` per tested package; under the RTK hook it shows one `Go test: <n> passed in <m> packages` line.

- [ ] **Step 8: Integration suite (envtest)**

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race`
Expected: `ok  	github.com/jacaudi/wireguard-operator/internal/controller` after roughly 90 s; no `FAIL`. Known timing flake: if the only failure is the spec `no update churn applies each owned object at most once per reconcile and persists nothing` with the message `a reconcile was still in flight`, rerun the suite once. It failed 1 run in about 15 during this plan's verification, passed 3/3 on rerun of the same commit, and did not reproduce in the plan gate's 23 runs. A second failure, or any other failing spec, is real.

- [ ] **Step 9: Lint**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...`
Expected: the last line is exactly `0 issues.` Gate on that line, never on the exit status (the RTK hook swallows it).

- [ ] **Step 10: Format check (CI's `gofmt / goimports check`)**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint fmt --diff; echo fmt-exit=$?`
Expected: only `fmt-exit=0`. Any diff printed above it fails CI's required `ci` check even when lint says `0 issues.` (the lint ratchet does not see formatting); run `gofmt -w` on the files it names, rerun, and keep the result in this task's commit.

- [ ] **Step 11: YAML lint**

Run: `yamllint .`
Expected: no line containing `error`. One pre-existing `warning` for `./taskfile.yml` 52:3 is normal.

- [ ] **Step 12: Commit**

```bash
git add -A -- api config internal release.yaml
git commit -m "refactor!: delete the wstunnel sidecar" -m "wstunnel obfuscated external-client traffic; in-cluster clients reach the server over its ClusterIP (design 2.2)." -m "BREAKING CHANGE: spec.tunnel, status.tunnel and the wstunnel sidecar and Service port are removed."
```

- [ ] **Step 13: Confirm the tree is clean and the commit is signed**

Run: `git status --porcelain`
Expected: no output. If anything is listed, it is a path this task touched but the `git add` above missed: `git add` it, `git commit --amend --no-edit`, rerun this step. A second non-empty result is `PLAN: BLOCKED — every path is a guess: dirty tree after commit`.

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. Any other value, a signing error, or a hang at the commit step is `PLAN: BLOCKED` (never `--no-gpg-sign`).

- [ ] **Step 14: Generated files and release.yaml still match their sources**

Run: `make -s manifests generate && git status --porcelain`
Expected: no output. If files are listed, the regenerate block (Task 2 Step 5) was skipped or an API edit came after it: `git add -A -- api config && git commit --amend --no-edit`, rerun. A second failure is `PLAN: BLOCKED — every path is a guess: codegen drift`.

Run: `sh hack/release-file-drift.sh`
Expected: the last line is `release.yaml matches config/`. If not, rerun the regenerate block (Task 2 Step 5 shows it), `git add release.yaml && git commit --amend --no-edit`, rerun this check. A second failure is `PLAN: BLOCKED — every path is a guess: release.yaml drift`.

---

### Task 4: Delete IPv6 and the per-peer policy fields

Kind: refactor

**Files:**
- Modify: `api/v1alpha1/wireguard_types.go` (`PeerCIDRv6`, `IPv6Only`), `api/v1alpha1/wireguardpeer_types.go` (`AddressV6`, `Disabled`, `EgressNetworkPolicies` and its four types and constants, `DownloadSpeed`, `UploadSpeed`, `Speed`), `internal/ipam/allocator.go` (`GetUsedIPv6IPs`), `internal/ipam/allocator_test.go`, `internal/controller/wireguard_controller.go` (`effectivePeerCIDR6`, v6 allocation, v6 `AllowedIPs`), `internal/controller/wireguardpeer_controller.go` (v6 duplicate check), `internal/agent/agent.go` (v6 branches of `IsStateValid`), `internal/wireguard/wireguard.go` (v6 address/route, v6 `AllowedIPs`, `Disabled`, v6 gateway derivation), `internal/controller/wireguard_controller_test.go`
- Regenerated: deepcopy, both CRDs, `release.yaml`

**Interfaces:**
- Consumes: Task 3's tree.
- Produces: `effectivePeerCIDR4(wg *v1alpha1.Wireguard) string` returns `wg.Spec.PeerCIDR` or `ipam.DefaultPeerCIDR4`, never empty; `gatewayIPFromPrefix` accepts IPv4 only; `(*ipam.Allocator).GetUsedIPv6IPs` no longer exists. Task 5 relies on `updateWireguardPeers` having no IPv6 branch (gocyclo).

IPv6 never worked (design §2.2) and is off unless `peerCIDRv6` is set; the policy fields were consumed only by the iptables code Task 1 deleted. Refactor rule: no new failing test.

- [ ] **Step 1: Remove the IPv6 specs and the IPv6 allocator test**

```bash
perl -i -ne 'print unless /^\t\tIt\("allocates both IPv4 and IPv6 addresses in dual-stack mode", func\(\) \{$/ .. /^\t\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t\tIt\("allocates only IPv6 addresses when ipv6Only is true", func\(\) \{$/ .. /^\t\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^func TestGetUsedIPv6IPs\(t \*testing\.T\) \{$/ .. /^\}$/' internal/ipam/allocator_test.go
gofmt -w internal/controller/wireguard_controller_test.go internal/ipam/allocator_test.go
```

Run: `cat internal/controller/wireguard_controller_test.go internal/ipam/allocator_test.go | grep -cF -e 'addresses in dual-stack mode' -e 'when ipv6Only is true' -e 'func TestGetUsedIPv6IPs'`
Expected: `0`. A non-zero count means an anchor did not match (the file is not in the state this plan assumes): use the restart in Global Constraints; if it recurs, `PLAN: BLOCKED — every path is a guess: plan drift <task>/<step>`.

- [ ] **Step 2: Adapt the custom-CIDR spec**

```bash
awk -v id=t4-test '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t4-test -->
~~~~diff
diff --git a/internal/controller/wireguard_controller_test.go b/internal/controller/wireguard_controller_test.go
index 2e5311d..e67af2c 100644
--- a/internal/controller/wireguard_controller_test.go
+++ b/internal/controller/wireguard_controller_test.go
@@ -998,7 +998,7 @@ var _ = Describe("wireguard controller", func() {
 
 		})
 
-		It("allocates IPv4 peer addresses from custom PeerCIDR in IPv4-only mode", func() {
+		It("allocates peer addresses from a custom PeerCIDR", func() {
 			wgKey := types.NamespacedName{
 				Name:      wgName,
 				Namespace: wgNamespace,
@@ -1067,11 +1067,6 @@ var _ = Describe("wireguard controller", func() {
 				Expect(k8sClient.Get(context.Background(), peerKey, peer)).Should(Succeed())
 				return peer.Spec.Address
 			}, Timeout, Interval).Should(Equal("10.9.0.2"))
-
-			Eventually(func() string {
-				Expect(k8sClient.Get(context.Background(), peerKey, peer)).Should(Succeed())
-				return peer.Spec.AddressV6
-			}, Timeout, Interval).Should(Equal(""))
 		})
 
 		It("reconciles deployment tolerations when spec changes", func() {
~~~~

- [ ] **Step 3: Delete IPv6 and the policy fields**

```bash
awk -v id=t4-impl '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t4-impl -->
~~~~diff
diff --git a/api/v1alpha1/wireguard_types.go b/api/v1alpha1/wireguard_types.go
index 2741574..cf3df33 100644
--- a/api/v1alpha1/wireguard_types.go
+++ b/api/v1alpha1/wireguard_types.go
@@ -40,15 +40,8 @@ type WireguardSpec struct {
 	// When set, this is used for peer endpoints instead of Address or Service status.
 	ExternalAddress string `json:"externalAddress,omitempty"`
 	// PeerCIDR is the IPv4 CIDR range from which Wireguard peer addresses will be allocated.
-	// If omitted (and IPv6Only is false), the operator will default to 10.8.0.0/24.
+	// If omitted, the operator will default to 10.8.0.0/24.
 	PeerCIDR string `json:"peerCIDR,omitempty"`
-	// PeerCIDRv6 is the IPv6 CIDR range from which Wireguard peer IPv6 addresses will be allocated.
-	// When set, IPv6 support is enabled for this Wireguard instance.
-	PeerCIDRv6 string `json:"peerCIDRv6,omitempty"`
-	// IPv6Only indicates that only IPv6 connectivity should be configured for peers.
-	// When true and PeerCIDRv6 is set, the operator will not allocate new IPv4 peer addresses
-	// or configure IPv4 routing/NAT for this instance.
-	IPv6Only bool `json:"ipv6Only,omitempty"`
 	// A string field that specifies the DNS server(s) to be used by the peers.
 	Dns string `json:"dns,omitempty"`
 	// A string field that specifies the DNS search domain(s) to be used by the peers.
diff --git a/api/v1alpha1/wireguardpeer_types.go b/api/v1alpha1/wireguardpeer_types.go
index 3e642a2..eae3700 100644
--- a/api/v1alpha1/wireguardpeer_types.go
+++ b/api/v1alpha1/wireguardpeer_types.go
@@ -37,12 +37,8 @@ type WireguardPeerSpec struct {
 	// Important: Run "make" to regenerate code after modifying this file
 	// The address of the peer.
 	Address string `json:"address,omitempty"`
-	// The IPv6 address of the peer.
-	AddressV6 string `json:"addressV6,omitempty"`
 	// The AllowedIPs of the peer.
 	AllowedIPs string `json:"allowedIPs,omitempty"`
-	// Set to true to temporarily disable the peer.
-	Disabled bool `json:"disabled,omitempty"`
 	// The DNS configuration for the peer.
 	Dns string `json:"dns,omitempty"`
 	// The DNS search domain(s) for the peer.
@@ -55,44 +51,6 @@ type WireguardPeerSpec struct {
 	//+kubebuilder:validation:Required
 	//+kubebuilder:validation:MinLength=1
 	WireguardRef string `json:"wireguardRef"`
-	// Egress network policies for the peer.
-	EgressNetworkPolicies EgressNetworkPolicies `json:"egressNetworkPolicies,omitempty"`
-	DownloadSpeed         Speed                 `json:"downloadSpeed,omitempty"`
-	UploadSpeed           Speed                 `json:"uploadSpeed,omitempty"`
-}
-
-type EgressNetworkPolicies []EgressNetworkPolicy
-
-// +kubebuilder:validation:Enum=ACCEPT;REJECT;Accept;Reject
-type EgressNetworkPolicyAction string
-
-// +kubebuilder:validation:Enum=TCP;UDP;ICMP
-type EgressNetworkPolicyProtocol string
-
-const (
-	EgressNetworkPolicyActionAccept EgressNetworkPolicyAction = "Accept"
-	EgressNetworkPolicyActionDeny   EgressNetworkPolicyAction = "Reject"
-)
-
-const (
-	EgressNetworkPolicyProtocolTCP EgressNetworkPolicyProtocol = "TCP"
-	EgressNetworkPolicyProtocolUDP EgressNetworkPolicyProtocol = "UDP"
-)
-
-type EgressNetworkPolicy struct {
-	// Specifies the action to take when outgoing traffic from a Wireguard peer matches the policy. This could be 'Accept' or 'Reject'.
-	Action EgressNetworkPolicyAction `json:"action,omitempty"`
-	// A struct that specifies the destination address and port for the traffic. This could include IP addresses or hostnames, as well as specific port numbers or port ranges.
-	To EgressNetworkPolicyTo `json:"to,omitempty"`
-	// Specifies the protocol to match for this policy. This could be TCP, UDP, or ICMP.
-	Protocol EgressNetworkPolicyProtocol `json:"protocol,omitempty"`
-}
-
-type EgressNetworkPolicyTo struct {
-	// A string field that specifies the destination IP address for traffic that matches the policy.
-	Ip string `json:"ip,omitempty"`
-	// An integer field that specifies the destination port number for traffic that matches the policy.
-	Port int32 `json:"port,omitempty" protobuf:"varint,3,opt,name=port"`
 }
 
 // WireguardPeerStatus defines the observed state of WireguardPeer
@@ -112,13 +70,6 @@ type WireguardPeerStatus struct {
 	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
 }
 
-type Speed struct {
-	Value int `json:"config,omitempty"`
-
-	// +kubebuilder:validation:Enum=mbps;kbps
-	Unit string `json:"unit,omitempty"`
-}
-
 //+kubebuilder:object:root=true
 //+kubebuilder:subresource:status
 //+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
diff --git a/internal/agent/agent.go b/internal/agent/agent.go
index c758c09..9a4713c 100644
--- a/internal/agent/agent.go
+++ b/internal/agent/agent.go
@@ -30,10 +30,8 @@ func IsStateValid(state State) error {
 		return fmt.Errorf("server private key should be of length 44")
 	}
 
-	if !state.Server.Spec.IPv6Only || state.Server.Spec.PeerCIDRv6 == "" {
-		if state.Server.Status.Address == "" {
-			return fmt.Errorf("server address is not defined")
-		}
+	if state.Server.Status.Address == "" {
+		return fmt.Errorf("server address is not defined")
 	}
 
 	if state.Server.Status.Dns == "" {
@@ -41,14 +39,8 @@ func IsStateValid(state State) error {
 	}
 
 	for i, peer := range state.Peers {
-		if state.Server.Spec.IPv6Only && state.Server.Spec.PeerCIDRv6 != "" {
-			if peer.Spec.AddressV6 == "" {
-				return fmt.Errorf("peer with index %d does not have the IPv6 address defined", i)
-			}
-		} else {
-			if peer.Spec.Address == "" {
-				return fmt.Errorf("peer with index %d does not have the address defined", i)
-			}
+		if peer.Spec.Address == "" {
+			return fmt.Errorf("peer with index %d does not have the address defined", i)
 		}
 
 		if peer.Spec.PublicKey == "" {
diff --git a/internal/controller/wireguard_controller.go b/internal/controller/wireguard_controller.go
index f284cf6..99dbc98 100644
--- a/internal/controller/wireguard_controller.go
+++ b/internal/controller/wireguard_controller.go
@@ -176,26 +176,13 @@ func getAvaialbleIp(cidr string, usedIps []string) (string, error) {
 }
 
 // effectivePeerCIDR4 returns the effective IPv4 peer CIDR for a Wireguard instance.
-// If IPv6Only is true and an IPv6 CIDR is configured, IPv4 CIDR is disabled (empty).
 func effectivePeerCIDR4(wg *v1alpha1.Wireguard) string {
-	if wg.Spec.IPv6Only && wg.Spec.PeerCIDRv6 != "" {
-		return ""
-	}
 	if wg.Spec.PeerCIDR != "" {
 		return wg.Spec.PeerCIDR
 	}
 	return ipam.DefaultPeerCIDR4
 }
 
-// effectivePeerCIDR6 returns the configured IPv6 peer CIDR and a boolean indicating
-// whether IPv6 is enabled for this Wireguard instance.
-func effectivePeerCIDR6(wg *v1alpha1.Wireguard) (string, bool) {
-	if wg.Spec.PeerCIDRv6 == "" {
-		return "", false
-	}
-	return wg.Spec.PeerCIDRv6, true
-}
-
 // persistentKeepalive returns the keepalive interval to write into peer configs.
 // A nil value means the field was absent — either the object predates the field or
 // defaulting is disabled — so fall back to the default rather than to zero, which
@@ -220,20 +207,8 @@ func (r *WireguardReconciler) updateWireguardPeers(ctx context.Context, req ctrl
 		return err
 	}
 
-	// Prepare used IP lists for IPv4 and IPv6 based on the effective CIDRs.
-	var usedIps4 []string
-	var usedIps6 []string
-
 	cidr4 := effectivePeerCIDR4(wireguard)
-	if cidr4 != "" {
-		usedIps4 = r.ipAllocator.GetUsedIPs(cidr4, peers)
-	}
-
-	cidr6, v6Enabled := effectivePeerCIDR6(wireguard)
-	if v6Enabled {
-		usedIps6 = r.ipAllocator.GetUsedIPv6IPs(cidr6, peers)
-	}
-	ipv6Only := wireguard.Spec.IPv6Only && v6Enabled
+	usedIps4 := r.ipAllocator.GetUsedIPs(cidr4, peers)
 
 	// Ensure or create the aggregated peer configs Secret
 	peerCfgSecretName := wireguard.Name + "-peer-configs"
@@ -264,8 +239,7 @@ func (r *WireguardReconciler) updateWireguardPeers(ctx context.Context, req ctrl
 	newPeerCfgData := map[string][]byte{}
 
 	for _, peer := range peers.Items {
-		// Allocate IPv4 address if enabled and not in IPv6-only mode.
-		if cidr4 != "" && !wireguard.Spec.IPv6Only && peer.Spec.Address == "" {
+		if peer.Spec.Address == "" {
 			ip, err := getAvaialbleIp(cidr4, usedIps4)
 			if err != nil {
 				return err
@@ -277,18 +251,6 @@ func (r *WireguardReconciler) updateWireguardPeers(ctx context.Context, req ctrl
 			usedIps4 = append(usedIps4, ip)
 		}
 
-		// Allocate IPv6 address if enabled.
-		if v6Enabled && peer.Spec.AddressV6 == "" {
-			ip6, err := getAvaialbleIp(cidr6, usedIps6)
-			if err != nil {
-				return err
-			}
-			peer.Spec.AddressV6 = ip6
-			if err := r.Update(ctx, &peer); err != nil {
-				return err
-			}
-			usedIps6 = append(usedIps6, ip6)
-		}
 		dnsConfiguration := dns
 
 		if dnsSearchDomain != "" {
@@ -298,19 +260,7 @@ func (r *WireguardReconciler) updateWireguardPeers(ctx context.Context, req ctrl
 		allowIps := peer.Spec.AllowedIPs
 
 		if allowIps == "" {
-			hasIPv4 := peer.Spec.Address != ""
-			hasIPv6 := peer.Spec.AddressV6 != ""
-
-			switch {
-			case ipv6Only && hasIPv6:
-				allowIps = "::/0"
-			case v6Enabled && hasIPv4 && hasIPv6:
-				allowIps = "0.0.0.0/0, ::/0"
-			case v6Enabled && !hasIPv4 && hasIPv6:
-				allowIps = "::/0"
-			default:
-				allowIps = "0.0.0.0/0"
-			}
+			allowIps = "0.0.0.0/0"
 		}
 
 		// Do not store shell-wrapped config in status anymore per upstream PR 212
@@ -324,9 +274,6 @@ func (r *WireguardReconciler) updateWireguardPeers(ctx context.Context, req ctrl
 				if peer.Spec.Address != "" {
 					addresses = append(addresses, peer.Spec.Address)
 				}
-				if peer.Spec.AddressV6 != "" {
-					addresses = append(addresses, peer.Spec.AddressV6)
-				}
 				addressLine := strings.Join(addresses, ", ")
 
 				pureCfg := fmt.Sprintf(`[Interface]
diff --git a/internal/controller/wireguardpeer_controller.go b/internal/controller/wireguardpeer_controller.go
index 27513f3..b9cb8cf 100644
--- a/internal/controller/wireguardpeer_controller.go
+++ b/internal/controller/wireguardpeer_controller.go
@@ -218,11 +218,11 @@ func (r *WireguardPeerReconciler) Reconcile(ctx context.Context, req ctrl.Reques
 	return ctrl.Result{}, nil
 }
 
-// checkDuplicateAddress checks whether the peer's address or addressV6 is already
+// checkDuplicateAddress checks whether the peer's address is already
 // used by another peer on the same wireguardRef. Returns a non-empty message
 // describing the conflict, or "" if no duplicate is found.
 func (r *WireguardPeerReconciler) checkDuplicateAddress(ctx context.Context, namespace string, peer *v1alpha1.WireguardPeer) (string, error) {
-	if peer.Spec.Address == "" && peer.Spec.AddressV6 == "" {
+	if peer.Spec.Address == "" {
 		return "", nil
 	}
 
@@ -234,12 +234,9 @@ func (r *WireguardPeerReconciler) checkDuplicateAddress(ctx context.Context, nam
 		if other.Name == peer.Name || other.Spec.WireguardRef != peer.Spec.WireguardRef {
 			continue
 		}
-		if peer.Spec.Address != "" && other.Spec.Address == peer.Spec.Address {
+		if other.Spec.Address == peer.Spec.Address {
 			return fmt.Sprintf("Duplicate address %s: already used by peer %s", peer.Spec.Address, other.Name), nil
 		}
-		if peer.Spec.AddressV6 != "" && other.Spec.AddressV6 == peer.Spec.AddressV6 {
-			return fmt.Sprintf("Duplicate IPv6 address %s: already used by peer %s", peer.Spec.AddressV6, other.Name), nil
-		}
 	}
 	return "", nil
 }
diff --git a/internal/ipam/allocator.go b/internal/ipam/allocator.go
index 0a42334..c075a7c 100644
--- a/internal/ipam/allocator.go
+++ b/internal/ipam/allocator.go
@@ -72,21 +72,6 @@ func (a *Allocator) GetUsedIPs(cidr string, peers *v1alpha1.WireguardPeerList) [
 	return usedIPs
 }
 
-// GetUsedIPv6IPs returns a list of IPv6 addresses that are currently in use by peers.
-// It includes the derived network and gateway addresses (first host) from the
-// provided IPv6 CIDR as reserved.
-func (a *Allocator) GetUsedIPv6IPs(cidr string, peers *v1alpha1.WireguardPeerList) []string {
-	usedIPs := deriveReservedIPs(cidr)
-
-	for _, peer := range peers.Items {
-		if peer.Spec.AddressV6 != "" {
-			usedIPs = append(usedIPs, peer.Spec.AddressV6)
-		}
-	}
-
-	return usedIPs
-}
-
 // deriveReservedIPs returns a slice containing the network address and the first
 // host address derived from the given CIDR. If the CIDR cannot be parsed, it
 // returns an empty slice and leaves validation to callers of AllocateIP.
diff --git a/internal/wireguard/wireguard.go b/internal/wireguard/wireguard.go
index 0b7f943..d869de9 100644
--- a/internal/wireguard/wireguard.go
+++ b/internal/wireguard/wireguard.go
@@ -252,57 +252,25 @@ func (wg *Wireguard) Sync(state agent.State) error {
 		return err
 	}
 
-	spec := state.Server.Spec
-
-	enableV6 := spec.PeerCIDRv6 != ""
-	ipv6Only := spec.IPv6Only && enableV6
-
-	// IPv4 configuration (skip in IPv6-only mode).
-	if !ipv6Only {
-		cidr4 := spec.PeerCIDR
-		if cidr4 == "" {
-			cidr4 = ipam.DefaultPeerCIDR4
-		}
-		if cidr4 != "" {
-			prefix4, err := netip.ParsePrefix(cidr4)
-			if err != nil {
-				return fmt.Errorf("failed to parse IPv4 CIDR %q: %w", cidr4, err)
-			}
-
-			addr4Net, gw4, err := gatewayIPFromPrefix(prefix4)
-			if err != nil {
-				return err
-			}
-
-			if err := syncAddress(wg.Iface, addr4Net, syscall.AF_INET); err != nil {
-				return err
-			}
-			if err := syncRoute(wg.Iface, cidr4, gw4, syscall.AF_INET); err != nil {
-				return err
-			}
-		}
+	cidr4 := state.Server.Spec.PeerCIDR
+	if cidr4 == "" {
+		cidr4 = ipam.DefaultPeerCIDR4
+	}
+	prefix4, err := netip.ParsePrefix(cidr4)
+	if err != nil {
+		return fmt.Errorf("failed to parse IPv4 CIDR %q: %w", cidr4, err)
 	}
 
-	// IPv6 configuration.
-	if enableV6 {
-		cidr6 := spec.PeerCIDRv6
-
-		prefix6, err := netip.ParsePrefix(cidr6)
-		if err != nil {
-			return fmt.Errorf("failed to parse IPv6 CIDR %q: %w", cidr6, err)
-		}
-
-		addr6Net, gw6, err := gatewayIPFromPrefix(prefix6)
-		if err != nil {
-			return err
-		}
+	addr4Net, gw4, err := gatewayIPFromPrefix(prefix4)
+	if err != nil {
+		return err
+	}
 
-		if err := syncAddress(wg.Iface, addr6Net, syscall.AF_INET6); err != nil {
-			return err
-		}
-		if err := syncRoute(wg.Iface, cidr6, gw6, syscall.AF_INET6); err != nil {
-			return err
-		}
+	if err := syncAddress(wg.Iface, addr4Net, syscall.AF_INET); err != nil {
+		return err
+	}
+	if err := syncRoute(wg.Iface, cidr4, gw4, syscall.AF_INET); err != nil {
+		return err
 	}
 
 	// sync wg configuration
@@ -324,28 +292,16 @@ func gatewayIPFromPrefix(prefix netip.Prefix) (*net.IPNet, net.IP, error) {
 	prefix = prefix.Masked()
 	addr := prefix.Addr()
 
-	switch {
-	case addr.Is4():
-		gw := addr.Next()
-		if !gw.Is4() {
-			return nil, nil, fmt.Errorf("failed to derive IPv4 gateway for prefix %q", prefix.String())
-		}
-		b := gw.As4()
-		ip := net.IPv4(b[0], b[1], b[2], b[3])
-		return &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}, ip, nil
-
-	case addr.Is6() && !addr.Is4():
-		gw := addr.Next()
-		if !gw.Is6() || gw.Is4() {
-			return nil, nil, fmt.Errorf("failed to derive IPv6 gateway for prefix %q", prefix.String())
-		}
-		b := gw.As16()
-		ip := net.IP(b[:])
-		return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, ip, nil
-
-	default:
+	if !addr.Is4() {
 		return nil, nil, fmt.Errorf("unsupported address family for prefix %q", prefix.String())
 	}
+	gw := addr.Next()
+	if !gw.Is4() {
+		return nil, nil, fmt.Errorf("failed to derive IPv4 gateway for prefix %q", prefix.String())
+	}
+	b := gw.As4()
+	ip := net.IPv4(b[0], b[1], b[2], b[3])
+	return &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}, ip, nil
 }
 
 func createPeersConfiguration(state agent.State, iface string) ([]wgtypes.PeerConfig, error) {
@@ -384,7 +340,7 @@ func createPeersConfiguration(state agent.State, iface string) ([]wgtypes.PeerCo
 			peerConfigurationByPublicKey[p.PublicKey.String()] = p
 
 		} else {
-			if peerState.Spec.Disabled || peerState.Spec.PublicKey == "" {
+			if peerState.Spec.PublicKey == "" {
 				// delete peer
 				p := wgtypes.PeerConfig{
 					Remove:     true,
@@ -397,9 +353,6 @@ func createPeersConfiguration(state agent.State, iface string) ([]wgtypes.PeerCo
 				if peerState.Spec.Address != "" {
 					desiredAllowed = append(desiredAllowed, getIP(peerState.Spec.Address+"/32")...)
 				}
-				if peerState.Spec.AddressV6 != "" {
-					desiredAllowed = append(desiredAllowed, getIP(peerState.Spec.AddressV6+"/128")...)
-				}
 
 				// Only update if AllowedIPs differ.
 				same := len(desiredAllowed) == len(peer.AllowedIPs)
@@ -426,14 +379,11 @@ func createPeersConfiguration(state agent.State, iface string) ([]wgtypes.PeerCo
 
 	// add new peers
 	for _, peer := range state.Peers {
-		if peer.Spec.Disabled {
-			continue
-		}
 		if peer.Spec.PublicKey == "" {
 			continue
 		}
 
-		if peer.Spec.Address == "" && peer.Spec.AddressV6 == "" {
+		if peer.Spec.Address == "" {
 			continue
 		}
 		key, err := wgtypes.ParseKey(peer.Spec.PublicKey)
@@ -447,13 +397,7 @@ func createPeersConfiguration(state agent.State, iface string) ([]wgtypes.PeerCo
 		}
 
 		// create peer
-		var allowed []net.IPNet
-		if peer.Spec.Address != "" {
-			allowed = append(allowed, getIP(peer.Spec.Address+"/32")...)
-		}
-		if peer.Spec.AddressV6 != "" {
-			allowed = append(allowed, getIP(peer.Spec.AddressV6+"/128")...)
-		}
+		allowed := getIP(peer.Spec.Address + "/32")
 		p := wgtypes.PeerConfig{
 			AllowedIPs: allowed,
 			PublicKey:  key,
~~~~

- [ ] **Step 4: Regenerate**

```bash
make manifests generate &&
AGENT=$(sed -n 's/^[[:space:]]*- --agent-image=//p' release.yaml | head -1) &&
MGR=$(sed -n 's/^        image: //p' release.yaml | head -1) &&
make generate-release-file AGENT_IMAGE="$AGENT" MANAGER_IMAGE="$MGR" > /dev/null &&
echo regen-ok
```

Expected: the block's last line is `regen-ok`. Anything else (a controller-gen or kustomize error) is `PLAN: BLOCKED — every path is a guess: regenerate`.

- [ ] **Step 5: No IPv6 or policy symbol survives**

Run: `grep -rn 'AddressV6\|PeerCIDRv6\|IPv6Only\|EgressNetworkPolic\|DownloadSpeed\|UploadSpeed\|Spec.Disabled' --include='*.go' api cmd internal ; echo grep-exit=$?`
Expected: only `grep-exit=1`.

- [ ] **Step 6: Build and vet every build tag**

Run: `go build ./... && go vet ./... && go vet -tags=integration ./... && go vet -tags=e2e ./...`
Expected: no output, exit 0. A compile error here means a patch step was skipped or reordered: use the restart in Global Constraints.

- [ ] **Step 7: Unit tests**

Run: `go test ./... -race -count=1`
Expected: no `FAIL` and no `[FAIL]` anywhere in the output. Raw output shows `ok` per tested package; under the RTK hook it shows one `Go test: <n> passed in <m> packages` line.

- [ ] **Step 8: Integration suite (envtest)**

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race`
Expected: `ok  	github.com/jacaudi/wireguard-operator/internal/controller` after roughly 90 s; no `FAIL`. Known timing flake: if the only failure is the spec `no update churn applies each owned object at most once per reconcile and persists nothing` with the message `a reconcile was still in flight`, rerun the suite once. It failed 1 run in about 15 during this plan's verification, passed 3/3 on rerun of the same commit, and did not reproduce in the plan gate's 23 runs. A second failure, or any other failing spec, is real.

- [ ] **Step 9: Lint**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...`
Expected: the last line is exactly `0 issues.` Gate on that line, never on the exit status (the RTK hook swallows it).

- [ ] **Step 10: Format check (CI's `gofmt / goimports check`)**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint fmt --diff; echo fmt-exit=$?`
Expected: only `fmt-exit=0`. Any diff printed above it fails CI's required `ci` check even when lint says `0 issues.` (the lint ratchet does not see formatting); run `gofmt -w` on the files it names, rerun, and keep the result in this task's commit.

- [ ] **Step 11: YAML lint**

Run: `yamllint .`
Expected: no line containing `error`. One pre-existing `warning` for `./taskfile.yml` 52:3 is normal.

- [ ] **Step 12: Commit**

```bash
git add -A -- api config internal release.yaml
git commit -m "refactor!: delete IPv6 and the per-peer policy fields" -m "The IPv6 path never worked and had no test; per-peer egress policy, speed and disabled had no consumer once the iptables sync went (design 2.2)." -m "BREAKING CHANGE: spec.peerCIDRv6, spec.ipv6Only and WireguardPeer spec.addressV6, disabled, egressNetworkPolicies, downloadSpeed and uploadSpeed are removed."
```

- [ ] **Step 13: Confirm the tree is clean and the commit is signed**

Run: `git status --porcelain`
Expected: no output. If anything is listed, it is a path this task touched but the `git add` above missed: `git add` it, `git commit --amend --no-edit`, rerun this step. A second non-empty result is `PLAN: BLOCKED — every path is a guess: dirty tree after commit`.

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. Any other value, a signing error, or a hang at the commit step is `PLAN: BLOCKED` (never `--no-gpg-sign`).

- [ ] **Step 14: Generated files and release.yaml still match their sources**

Run: `make -s manifests generate && git status --porcelain`
Expected: no output. If files are listed, the regenerate block (Task 2 Step 5) was skipped or an API edit came after it: `git add -A -- api config && git commit --amend --no-edit`, rerun. A second failure is `PLAN: BLOCKED — every path is a guess: codegen drift`.

Run: `sh hack/release-file-drift.sh`
Expected: the last line is `release.yaml matches config/`. If not, rerun the regenerate block (Task 2 Step 5 shows it), `git add release.yaml && git commit --amend --no-edit`, rerun this check. A second failure is `PLAN: BLOCKED — every path is a guess: release.yaml drift`.

---

### Task 5: Delete peer config rendering and peer key minting

Kind: refactor

**Files:**
- Modify: `api/v1alpha1/wireguard_types.go` (`Mtu`, `Dns`, `DnsSearchDomain`, `PersistentKeepalive`, `Status.Dns`), `api/v1alpha1/wireguardpeer_types.go` (`PrivateKey` type and field, `AllowedIPs`, `Dns`, `DnsSearchDomain`), `internal/controller/wireguard_controller.go` (`defaultPersistentKeepalive`, `persistentKeepalive`, the `<name>-peer-configs` Secret, the kube-dns lookup), `internal/controller/wireguardpeer_controller.go` (key minting, `secretBuilder`, the discarded `wireguardSecret` read and its stale comment), `internal/resources/secret.go` (`ForPeer`), `internal/agent/agent.go` (`IsStateValid`'s `Status.Address` and `Status.Dns` checks), `internal/agent/agent_test.go`, `internal/controller/wireguard_controller_test.go`, `internal/it/it_test.go`, `examples/server.yaml`, `examples/peer.yaml`
- Regenerated: deepcopy, both CRDs, `release.yaml`

**Interfaces:**
- Consumes: Task 4's IPv4-only `updateWireguardPeers` and `effectivePeerCIDR4`.
- Produces:
  - `func (r *WireguardReconciler) updateWireguardPeers(ctx context.Context, req ctrl.Request, wireguard *v1alpha1.Wireguard) error` allocates `spec.address` to every peer that has a `spec.publicKey` but no address, and sets that peer's `status.status` to `ready` / `status.message` to `Peer configured`, removing any `Ready` condition (a stale `PublicKeyMissing` from when it was keyless). It writes when `status.status` is not `ready` or a `Ready` condition is present.
  - `func (r *WireguardReconciler) markPeerPublicKeyMissing(ctx context.Context, peer *v1alpha1.WireguardPeer) error` handles a peer with no `publicKey`: no address; `status.status` `pending`, `status.message` `Waiting for spec.publicKey`, condition `Ready=False` reason `PublicKeyMissing`. It writes only when that differs from what is recorded (Ruling 11).
  - Test helper `newPublicKey() string` at the end of `internal/controller/wireguard_controller_test.go`; every spec that expects a peer to be configured now gives it a key.
  - The `api/v1alpha1/wireguard_types.go` struct is left gofmt-aligned by the patch. `IsStateValid(state State) error` checks only the 44-character server private key and each peer's `spec.address` and `spec.publicKey`. `(*SecretBuilder).ForPeer` and `v1alpha1.PrivateKey` no longer exist.

- [ ] **Step 1: Remove the specs that only exercised rendered configs, keepalive, DNS and minting**

```bash
perl -i -ne 'print unless /^\/\/ peerConfigLines returns the lines of a rendered peer config/ .. /^\}$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t\tIt\("sets Custom address for peers through Wireguard\.Spec\.Address", func\(\) \{$/ .. /^\t\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t\tIt\("sets Custom DNS through Wireguard\.Spec\.DNS", func\(\) \{$/ .. /^\t\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t\tIt\("uses address for the Service and externalAddress for peer endpoints", func\(\) \{$/ .. /^\t\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t\tIt\("uses externalAddress for peer endpoints when address is empty", func\(\) \{$/ .. /^\t\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t\tIt\("sets DNS search domain override via Wireguard\.Spec\.DnsSearchDomain", func\(\) \{$/ .. /^\t\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\tContext\("PersistentKeepalive", func\(\) \{$/ .. /^\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\tContext\("Race condition: WireguardPeer spec completeness", func\(\) \{$/ .. /^\t\}\)$/' internal/controller/wireguard_controller_test.go
gofmt -w internal/controller/wireguard_controller_test.go
```

Run: `cat internal/controller/wireguard_controller_test.go | grep -cF -e 'func peerConfigLines' -e 'sets Custom address for peers' -e 'sets Custom DNS through' -e 'externalAddress for peer endpoints' -e 'sets DNS search domain override' -e 'Context("PersistentKeepalive"' -e 'Context("Race condition'`
Expected: `0`. A non-zero count means an anchor did not match (the file is not in the state this plan assumes): use the restart in Global Constraints; if it recurs, `PLAN: BLOCKED — every path is a guess: plan drift <task>/<step>`.

- [ ] **Step 2: Write the failing tests and adapt the survivors**

The patch adds three tests:
- `TestIsStateValidNeedsNoServerAddressOrDNS`, the red unit test.
- `TestIsStateValidAcceptsAStateWrittenBeforeTheTrim`, Review Focus 1. It is a regression guard and passes before and after.
- The envtest spec "allocates an address to a publicKey peer and writes no client config", which covers Review Focus 2: its keyless peer must report `Ready=False` reason `PublicKeyMissing`, get no address, and stay out of `state.json`; once given a key it must get an address and `ready` with no `Ready` condition left. It is the red spec.

The patch also drops `Status.Dns` from the remaining summary assertions, removes kube-dns from `BeforeEach`, gives every peer that must be configured a `publicKey` (helper `newPublicKey`), and gives the e2e peer a `publicKey`. It also deletes the ClusterIP spec's two `//nolint:errcheck` directives and fixes its `//get peer secret` comments to `// get peer secret`: the trim makes the lint ratchet count those lines as changed, and it reports them (nolintlint, revive comment-spacings).

```bash
awk -v id=t5-test '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t5-test -->
~~~~diff
diff --git a/internal/agent/agent_test.go b/internal/agent/agent_test.go
index 2e6c845..e983c82 100644
--- a/internal/agent/agent_test.go
+++ b/internal/agent/agent_test.go
@@ -91,6 +91,36 @@ func TestGetDesiredStateDecodesState(t *testing.T) {
 	}
 }
 
+func TestIsStateValidNeedsNoServerAddressOrDNS(t *testing.T) {
+	// A ClusterIP server has no external address and pushes no DNS, so a state whose
+	// server status carries neither must still be applied.
+	state := State{ServerPrivateKey: "PLA+byaz6a5jbnioieB4NEELG7sdq7Wk+3u9Kw0s6zk="}
+	if err := IsStateValid(state); err != nil {
+		t.Errorf("IsStateValid(state without address or DNS) = %v, want nil", err)
+	}
+}
+
+func TestIsStateValidAcceptsAStateWrittenBeforeTheTrim(t *testing.T) {
+	// During a rollout the new agent can read a state.json written by an operator that still
+	// rendered the deleted fields. Unknown fields must be ignored, not rejected.
+	const legacy = `{"Server":{"spec":{"mtu":"1380","serviceType":"LoadBalancer","dns":"1.1.1.1"},` +
+		`"status":{"address":"203.0.113.10","dns":"10.0.0.10","port":"51820"}},` +
+		`"ServerPrivateKey":"PLA+byaz6a5jbnioieB4NEELG7sdq7Wk+3u9Kw0s6zk=",` +
+		`"Peers":[{"spec":{"address":"10.8.0.2","publicKey":"v5Q+oRNBfxafhiAR9bXGEhRsRFfEFb7XsFV8DWGrykQ=",` +
+		`"wireguardRef":"vpn","privateKeyRef":{"secretKeyRef":{"name":"p","key":"privateKey"}}}}]}`
+
+	state, _, err := GetDesiredState(writeState(t, legacy))
+	if err != nil {
+		t.Fatalf("GetDesiredState(pre-trim state): %v", err)
+	}
+	if err := IsStateValid(state); err != nil {
+		t.Errorf("IsStateValid(pre-trim state) = %v, want nil", err)
+	}
+	if len(state.Peers) != 1 || state.Peers[0].Spec.Address != "10.8.0.2" {
+		t.Errorf("peers = %+v, want the single pre-trim peer at 10.8.0.2", state.Peers)
+	}
+}
+
 func TestGetDesiredStateReportsMissingFile(t *testing.T) {
 	if _, _, err := GetDesiredState(filepath.Join(t.TempDir(), "absent.json")); err == nil {
 		t.Error("expected an error for a missing state file, got nil")
diff --git a/internal/controller/wireguard_controller_test.go b/internal/controller/wireguard_controller_test.go
index 11bf76e..0efdf8b 100644
--- a/internal/controller/wireguard_controller_test.go
+++ b/internal/controller/wireguard_controller_test.go
@@ -7,15 +7,17 @@ import (
 	"fmt"
 	"slices"
 	"strconv"
-	"strings"
 	"time"
 
 	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
 	"github.com/jacaudi/wireguard-operator/internal/resources"
 	. "github.com/onsi/ginkgo"
 	. "github.com/onsi/gomega"
+	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"
 	appsv1 "k8s.io/api/apps/v1"
 	corev1 "k8s.io/api/core/v1"
+	apierrors "k8s.io/apimachinery/pkg/api/errors"
+	"k8s.io/apimachinery/pkg/api/meta"
 	"k8s.io/apimachinery/pkg/api/resource"
 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 	"k8s.io/apimachinery/pkg/types"
@@ -103,11 +105,10 @@ var _ = Describe("wireguard controller", func() {
 
 	// Define utility constants for object names and testing timeouts/durations and intervals.
 	const (
-		wgName       = "vpn"
-		wgNamespace  = "default"
-		Timeout      = time.Second * 10
-		Interval     = time.Millisecond * 250
-		dnsServiceIp = "10.0.0.42"
+		wgName      = "vpn"
+		wgNamespace = "default"
+		Timeout     = time.Second * 10
+		Interval    = time.Millisecond * 250
 	)
 
 	wgKey := types.NamespacedName{
@@ -158,22 +159,110 @@ var _ = Describe("wireguard controller", func() {
 		for _, c := range cList.Items {
 			Expect(k8sClient.Delete(context.Background(), &c)).Should(Succeed())
 		}
-
-		// create kube-dns service
-		dnsService := &corev1.Service{
-			ObjectMeta: metav1.ObjectMeta{
-				Name:      "kube-dns",
-				Namespace: "kube-system",
-			},
-			Spec: corev1.ServiceSpec{
-				ClusterIP: dnsServiceIp,
-				Ports:     []corev1.ServicePort{{Name: "dns", Protocol: corev1.ProtocolUDP, Port: 53}},
-			},
-		}
-		Expect(k8sClient.Create(context.Background(), dnsService)).Should(Succeed())
-
 	})
 	Context("Wireguard", func() {
+		It("allocates an address to a publicKey peer and writes no client config", func() {
+			wgServer := &v1alpha1.Wireguard{
+				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
+				Spec:       v1alpha1.WireguardSpec{ServiceType: corev1.ServiceTypeClusterIP},
+			}
+			Expect(k8sClient.Create(context.Background(), wgServer)).Should(Succeed())
+
+			key, err := wgtypes.GeneratePrivateKey()
+			Expect(err).ShouldNot(HaveOccurred())
+			publicKey := key.PublicKey().String()
+
+			peerKey := types.NamespacedName{Name: wgName + "-pod-peer", Namespace: wgNamespace}
+			peer := &v1alpha1.WireguardPeer{
+				ObjectMeta: metav1.ObjectMeta{Name: peerKey.Name, Namespace: peerKey.Namespace},
+				Spec:       v1alpha1.WireguardPeerSpec{WireguardRef: wgName, PublicKey: publicKey},
+			}
+			Expect(k8sClient.Create(context.Background(), peer)).Should(Succeed())
+
+			Eventually(func() string {
+				got := &v1alpha1.WireguardPeer{}
+				if err := k8sClient.Get(context.Background(), peerKey, got); err != nil {
+					return ""
+				}
+				return got.Spec.Address
+			}, Timeout, Interval).Should(Equal("10.8.0.2"))
+
+			// A peer without a publicKey is never minted a key, never allocated an address and
+			// never handed to the server: it reports Ready=False with reason PublicKeyMissing.
+			keylessKey := types.NamespacedName{Name: wgName + "-keyless-peer", Namespace: wgNamespace}
+			keyless := &v1alpha1.WireguardPeer{
+				ObjectMeta: metav1.ObjectMeta{Name: keylessKey.Name, Namespace: keylessKey.Namespace},
+				Spec:       v1alpha1.WireguardPeerSpec{WireguardRef: wgName},
+			}
+			Expect(k8sClient.Create(context.Background(), keyless)).Should(Succeed())
+			Eventually(func() string {
+				got := &v1alpha1.WireguardPeer{}
+				if err := k8sClient.Get(context.Background(), keylessKey, got); err != nil {
+					return ""
+				}
+				c := meta.FindStatusCondition(got.Status.Conditions, ConditionReady)
+				if c == nil || c.Status != metav1.ConditionFalse {
+					return "no-ready-false-condition"
+				}
+				return c.Reason
+			}, Timeout, Interval).Should(Equal("PublicKeyMissing"))
+			Consistently(func() string {
+				got := &v1alpha1.WireguardPeer{}
+				if err := k8sClient.Get(context.Background(), keylessKey, got); err != nil {
+					return "get-failed"
+				}
+				return got.Spec.Address + "|" + got.Status.Status
+			}, "2s", Interval).Should(Equal("|"+v1alpha1.Pending), "a keyless peer gets no address and is never Ready")
+
+			// The server Secret's state.json carries the keyed peer: that is all the agent needs.
+			stateJSON := func() string {
+				secret := &corev1.Secret{}
+				if err := k8sClient.Get(context.Background(), wgKey, secret); err != nil {
+					return ""
+				}
+				return string(secret.Data["state.json"])
+			}
+			Eventually(stateJSON, Timeout, Interval).Should(ContainSubstring(publicKey))
+			Consistently(stateJSON, "2s", Interval).ShouldNot(ContainSubstring(keylessKey.Name),
+				"a peer without a publicKey must not reach the server")
+
+			// No client configuration and no peer key material is ever written.
+			Consistently(func() bool {
+				err := k8sClient.Get(context.Background(),
+					types.NamespacedName{Name: wgName + "-peer-configs", Namespace: wgNamespace}, &corev1.Secret{})
+				return apierrors.IsNotFound(err)
+			}, "2s", Interval).Should(BeTrue(), "no aggregated peer-config Secret may exist")
+			for _, name := range []string{peerKey.Name, keylessKey.Name} {
+				err = k8sClient.Get(context.Background(),
+					types.NamespacedName{Name: name + "-peer", Namespace: wgNamespace}, &corev1.Secret{})
+				Expect(apierrors.IsNotFound(err)).Should(BeTrue(), "no per-peer key Secret may exist for %s", name)
+			}
+
+			// Once the keyless peer supplies a key it is admitted: it gets an address, becomes
+			// Ready, and the PublicKeyMissing condition is cleared rather than left behind.
+			Eventually(func() error {
+				got := &v1alpha1.WireguardPeer{}
+				if err := k8sClient.Get(context.Background(), keylessKey, got); err != nil {
+					return err
+				}
+				got.Spec.PublicKey = newPublicKey()
+				return k8sClient.Update(context.Background(), got)
+			}, Timeout, Interval).Should(Succeed())
+			Eventually(func() string {
+				got := &v1alpha1.WireguardPeer{}
+				if err := k8sClient.Get(context.Background(), keylessKey, got); err != nil {
+					return "get-failed"
+				}
+				if c := meta.FindStatusCondition(got.Status.Conditions, ConditionReady); c != nil {
+					return "condition Ready=" + string(c.Status) + "/" + c.Reason
+				}
+				if got.Spec.Address == "" {
+					return "no-address"
+				}
+				return got.Status.Status
+			}, Timeout, Interval).Should(Equal(v1alpha1.Ready), "a peer that gains a key is admitted and its PublicKeyMissing condition cleared")
+		})
+
 		It("Should create a WG with ServiceType NodePort and WG peer successfully", func() {
 			var expectedNodePort = "30000"
 			expectedAddress := "69.0.0.2"
@@ -232,8 +321,8 @@ var _ = Describe("wireguard controller", func() {
 				if err != nil {
 					return []string{"", "", "", "", ""}
 				}
-				return []string{wg.Status.Address, wg.Status.Port, wg.Status.Dns, wg.Status.Status, wg.Status.Message}
-			}, Timeout, Interval).Should(Equal([]string{expectedAddress, expectedNodePort, dnsServiceIp, "ready", "VPN is active!"}))
+				return []string{wg.Status.Address, wg.Status.Port, wg.Status.Status, wg.Status.Message}
+			}, Timeout, Interval).Should(Equal([]string{expectedAddress, expectedNodePort, "ready", "VPN is active!"}))
 
 			Eventually(func() string {
 				deploymentKey := types.NamespacedName{
@@ -258,11 +347,12 @@ var _ = Describe("wireguard controller", func() {
 				},
 				Spec: v1alpha1.WireguardPeerSpec{
 					WireguardRef: wgKey.Name,
+					PublicKey:    newPublicKey(),
 				},
 			}
 			Expect(k8sClient.Create(context.Background(), peer)).Should(Succeed())
 
-			//get peer secret
+			// get peer secret
 			wgSecretKeyName := types.NamespacedName{
 				Name:      wgKey.Name,
 				Namespace: wgKey.Namespace,
@@ -400,8 +490,8 @@ var _ = Describe("wireguard controller", func() {
 				if err != nil {
 					return []string{"", "", "", "", ""}
 				}
-				return []string{wg.Status.Address, wg.Status.Port, wg.Status.Dns, wg.Status.Status, wg.Status.Message}
-			}, Timeout, Interval).Should(Equal([]string{expectedExternalHostName, "51820", dnsServiceIp, "ready", "VPN is active!"}))
+				return []string{wg.Status.Address, wg.Status.Port, wg.Status.Status, wg.Status.Message}
+			}, Timeout, Interval).Should(Equal([]string{expectedExternalHostName, "51820", "ready", "VPN is active!"}))
 
 			// Condition should reflect Ready
 			Eventually(func() string {
@@ -438,11 +528,12 @@ var _ = Describe("wireguard controller", func() {
 				},
 				Spec: v1alpha1.WireguardPeerSpec{
 					WireguardRef: wgKey.Name,
+					PublicKey:    newPublicKey(),
 				},
 			}
 			Expect(k8sClient.Create(context.Background(), peer)).Should(Succeed())
 
-			//get peer secret
+			// get peer secret
 			wgSecretKeyName := types.NamespacedName{
 				Name:      wgKey.Name,
 				Namespace: wgKey.Namespace,
@@ -538,7 +629,6 @@ var _ = Describe("wireguard controller", func() {
 			// match labels
 			Eventually(func() map[string]string {
 				svc := &corev1.Service{}
-				//nolint:errcheck
 				k8sClient.Get(context.Background(), serviceKey, svc)
 				return svc.Spec.Selector
 			}, Timeout, Interval).Should(BeEquivalentTo(expectedLabels))
@@ -546,7 +636,6 @@ var _ = Describe("wireguard controller", func() {
 			// match service type
 			Eventually(func() corev1.ServiceType {
 				svc := &corev1.Service{}
-				//nolint:errcheck
 				k8sClient.Get(context.Background(), serviceKey, svc)
 				return svc.Spec.Type
 			}, Timeout, Interval).Should(Equal(corev1.ServiceTypeClusterIP))
@@ -572,8 +661,8 @@ var _ = Describe("wireguard controller", func() {
 				if err != nil {
 					return []string{"", "", "", "", ""}
 				}
-				return []string{wg.Status.Address, wg.Status.Port, wg.Status.Dns, wg.Status.Status, wg.Status.Message}
-			}, Timeout, Interval).Should(Equal([]string{expectedAddress, "51820", dnsServiceIp, "ready", "VPN is active!"}))
+				return []string{wg.Status.Address, wg.Status.Port, wg.Status.Status, wg.Status.Message}
+			}, Timeout, Interval).Should(Equal([]string{expectedAddress, "51820", "ready", "VPN is active!"}))
 
 			Eventually(func() string {
 				deploymentKey := types.NamespacedName{
@@ -598,11 +687,12 @@ var _ = Describe("wireguard controller", func() {
 				},
 				Spec: v1alpha1.WireguardPeerSpec{
 					WireguardRef: wgKey.Name,
+					PublicKey:    newPublicKey(),
 				},
 			}
 			Expect(k8sClient.Create(context.Background(), peer)).Should(Succeed())
 
-			//get peer secret
+			// get peer secret
 			wgSecretKeyName := types.NamespacedName{
 				Name:      wgKey.Name,
 				Namespace: wgKey.Namespace,
@@ -669,7 +759,6 @@ var _ = Describe("wireguard controller", func() {
 			// match labels
 			Eventually(func() map[string]string {
 				svc := &corev1.Service{}
-				//nolint:errcheck
 				k8sClient.Get(context.Background(), serviceKey, svc)
 				return svc.Spec.Selector
 			}, Timeout, Interval).Should(BeEquivalentTo(expectedLabels))
@@ -677,7 +766,6 @@ var _ = Describe("wireguard controller", func() {
 			// match service type
 			Eventually(func() corev1.ServiceType {
 				svc := &corev1.Service{}
-				//nolint:errcheck
 				k8sClient.Get(context.Background(), serviceKey, svc)
 				return svc.Spec.Type
 			}, Timeout, Interval).Should(Equal(corev1.ServiceTypeClusterIP))
@@ -696,6 +784,7 @@ var _ = Describe("wireguard controller", func() {
 				},
 				Spec: v1alpha1.WireguardPeerSpec{
 					WireguardRef: wgKey.Name,
+					PublicKey:    newPublicKey(),
 				},
 			}
 			Expect(k8sClient.Create(context.Background(), peer)).Should(Succeed())
@@ -926,6 +1015,7 @@ var _ = Describe("wireguard controller", func() {
 				Spec: v1alpha1.WireguardPeerSpec{
 					WireguardRef: wgName,
 					Address:      "10.8.0.22",
+					PublicKey:    newPublicKey(),
 				},
 			}
 			Expect(k8sClient.Create(context.Background(), peer1)).Should(Succeed())
@@ -950,6 +1040,7 @@ var _ = Describe("wireguard controller", func() {
 				Spec: v1alpha1.WireguardPeerSpec{
 					WireguardRef: wgName,
 					Address:      "10.8.0.22",
+					PublicKey:    newPublicKey(),
 				},
 			}
 			Expect(k8sClient.Create(context.Background(), peer2)).Should(Succeed())
@@ -970,18 +1061,6 @@ var _ = Describe("wireguard controller", func() {
 				ContainSubstring("Duplicate address 10.8.0.22"),
 				ContainSubstring(peer1Key.Name),
 			))
-
-			// The duplicate peer should NOT appear in the peer-configs Secret
-			Eventually(func() bool {
-				secret := &corev1.Secret{}
-				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: wgName + "-peer-configs", Namespace: wgNamespace}, secret)
-				if err != nil {
-					return false
-				}
-				_, hasPeer1 := secret.Data[peer1Key.Name]
-				_, hasPeer2 := secret.Data[peer2Key.Name]
-				return hasPeer1 && !hasPeer2
-			}, Timeout, Interval).Should(BeTrue())
 		})
 
 		for _, useWgUserspace := range []bool{true, false} {
@@ -1331,3 +1410,11 @@ var _ = Describe("wireguard controller", func() {
 	})
 
 })
+
+// newPublicKey returns a fresh WireGuard public key. The operator no longer mints peer keys, so
+// every peer a spec expects to be configured must carry one.
+func newPublicKey() string {
+	key, err := wgtypes.GeneratePrivateKey()
+	Expect(err).ShouldNot(HaveOccurred())
+	return key.PublicKey().String()
+}
diff --git a/internal/it/it_test.go b/internal/it/it_test.go
index 810146a..c978781 100644
--- a/internal/it/it_test.go
+++ b/internal/it/it_test.go
@@ -16,7 +16,6 @@ kind: Wireguard
 metadata:
   name: vpn
 spec:
-  mtu: "1380"
   serviceType: "NodePort"`
 
 		// kubectl apply -f
@@ -31,6 +30,7 @@ metadata:
   name: peer20
 spec:
   wireguardRef: "vpn"
+  publicKey: "PLA+byaz6a5jbnioieB4NEELG7sdq7Wk+3u9Kw0s6zk="
 
 `
 		// kubectl apply -f
~~~~

- [ ] **Step 3: Watch the unit test fail**

Run: `go test ./internal/agent/ -run TestIsStateValid -count=1 -v`
Expected: `TestIsStateValidNeedsNoServerAddressOrDNS` FAILS with `IsStateValid(state without address or DNS) = server address is not defined, want nil`; `TestIsStateValidAcceptsAStateWrittenBeforeTheTrim` PASSES.

- [ ] **Step 4: Watch the envtest spec fail**

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -ginkgo.focus="writes no client config"`
Expected: FAIL (about 70 s) with `Expected <string>: no-ready-false-condition to equal <string>: PublicKeyMissing`. The old peer controller mints the keyless peer a key, so it becomes Ready instead.

- [ ] **Step 5: Delete rendering, minting, DNS, MTU and keepalive**

```bash
awk -v id=t5-impl '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t5-impl -->
~~~~diff
diff --git a/api/v1alpha1/wireguard_types.go b/api/v1alpha1/wireguard_types.go
index cf3df33..2c9514f 100644
--- a/api/v1alpha1/wireguard_types.go
+++ b/api/v1alpha1/wireguard_types.go
@@ -30,8 +30,6 @@ const (
 
 // WireguardSpec defines the desired state of Wireguard
 type WireguardSpec struct {
-	// A string field that specifies the maximum transmission unit (MTU) size for Wireguard packets for all peers.
-	Mtu string `json:"mtu,omitempty"`
 	// A string field that specifies the address for the Wireguard Service.
 	// When ServiceType is LoadBalancer, this can be used to request a fixed Service IP.
 	// If ExternalAddress is not set, this value is also used as the peer endpoint.
@@ -42,10 +40,6 @@ type WireguardSpec struct {
 	// PeerCIDR is the IPv4 CIDR range from which Wireguard peer addresses will be allocated.
 	// If omitted, the operator will default to 10.8.0.0/24.
 	PeerCIDR string `json:"peerCIDR,omitempty"`
-	// A string field that specifies the DNS server(s) to be used by the peers.
-	Dns string `json:"dns,omitempty"`
-	// A string field that specifies the DNS search domain(s) to be used by the peers.
-	DnsSearchDomain string `json:"dnsSearchDomain,omitempty"`
 	// A field that specifies the type of Kubernetes service that should be used for the Wireguard VPN. This could be ClusterIP, NodePort or LoadBalancer, depending on the needs of the deployment.
 	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
 	// A field that specifies the value to use for a nodePort ServiceType
@@ -55,18 +49,8 @@ type WireguardSpec struct {
 	// A boolean field that specifies whether IP forwarding should be enabled on the Wireguard VPN pod at startup. This can be useful to enable if the peers are having problems with sending traffic to the internet.
 	EnableIpForwardOnPodInit bool `json:"enableIpForwardOnPodInit,omitempty"`
 	// A boolean field that specifies whether to use the userspace implementation of Wireguard instead of the kernel one.
-	UseWgUserspaceImplementation bool `json:"useWgUserspaceImplementation,omitempty"`
-	// PersistentKeepalive is the interval in seconds at which peers send keepalive packets
-	// to the server. This keeps NAT and conntrack entries alive so the server can reach the
-	// peer, and makes the peer re-handshake promptly after the server pod is rescheduled.
-	// Set to 0 to disable. Defaults to 25.
-	// +kubebuilder:validation:Minimum=0
-	// +kubebuilder:validation:Maximum=65535
-	// +kubebuilder:default=25
-	// +optional
-	PersistentKeepalive *int32 `json:"persistentKeepalive,omitempty"`
-
-	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
+	UseWgUserspaceImplementation bool              `json:"useWgUserspaceImplementation,omitempty"`
+	NodeSelector                 map[string]string `json:"nodeSelector,omitempty"`
 	// A list of Kubernetes taint tolerations applied to the Wireguard pod.
 	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
 	Agent       WireguardPodSpec    `json:"agent,omitempty"`
@@ -81,7 +65,6 @@ type WireguardPodSpec struct {
 type WireguardStatus struct {
 	// A string field that specifies the address for the Wireguard VPN server that is currently being used.
 	Address string `json:"address,omitempty"`
-	Dns     string `json:"dns,omitempty"`
 	// A string field that specifies the port for the Wireguard VPN server that is currently being used.
 	Port string `json:"port,omitempty"`
 	// A string field that represents the current status of Wireguard. This could include values like ready, pending, or error.
diff --git a/api/v1alpha1/wireguardpeer_types.go b/api/v1alpha1/wireguardpeer_types.go
index eae3700..8353b7b 100644
--- a/api/v1alpha1/wireguardpeer_types.go
+++ b/api/v1alpha1/wireguardpeer_types.go
@@ -17,14 +17,9 @@ limitations under the License.
 package v1alpha1
 
 import (
-	corev1 "k8s.io/api/core/v1"
 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 )
 
-type PrivateKey struct {
-	SecretKeyRef corev1.SecretKeySelector `json:"secretKeyRef"`
-}
-
 type Status struct {
 }
 
@@ -37,14 +32,6 @@ type WireguardPeerSpec struct {
 	// Important: Run "make" to regenerate code after modifying this file
 	// The address of the peer.
 	Address string `json:"address,omitempty"`
-	// The AllowedIPs of the peer.
-	AllowedIPs string `json:"allowedIPs,omitempty"`
-	// The DNS configuration for the peer.
-	Dns string `json:"dns,omitempty"`
-	// The DNS search domain(s) for the peer.
-	DnsSearchDomain string `json:"dnsSearchDomain,omitempty"`
-	// The private key of the peer
-	PrivateKey PrivateKey `json:"privateKeyRef,omitempty"`
 	// The key used by the peer to authenticate with the wg server.
 	PublicKey string `json:"publicKey,omitempty"`
 	// The name of the Wireguard instance in k8s that the peer belongs to. The wg instance should be in the same namespace as the peer.
diff --git a/examples/peer.yaml b/examples/peer.yaml
index 04824e2..afcf9c7 100644
--- a/examples/peer.yaml
+++ b/examples/peer.yaml
@@ -4,4 +4,6 @@ metadata:
   name: peer20
 spec:
   wireguardRef: "vpn"
+  # The public half of a key the client generated itself: wg genkey | wg pubkey
+  publicKey: "v5Q+oRNBfxafhiAR9bXGEhRsRFfEFb7XsFV8DWGrykQ="
 
diff --git a/examples/server.yaml b/examples/server.yaml
index 2877cc6..796988a 100644
--- a/examples/server.yaml
+++ b/examples/server.yaml
@@ -2,5 +2,4 @@ apiVersion: vpn.wireguard-operator.io/v1alpha1
 kind: Wireguard
 metadata:
   name: vpn
-spec:
-  mtu: "1380"
+spec: {}
diff --git a/internal/agent/agent.go b/internal/agent/agent.go
index 9a4713c..2e8d002 100644
--- a/internal/agent/agent.go
+++ b/internal/agent/agent.go
@@ -30,14 +30,6 @@ func IsStateValid(state State) error {
 		return fmt.Errorf("server private key should be of length 44")
 	}
 
-	if state.Server.Status.Address == "" {
-		return fmt.Errorf("server address is not defined")
-	}
-
-	if state.Server.Status.Dns == "" {
-		return fmt.Errorf("dns is not defined")
-	}
-
 	for i, peer := range state.Peers {
 		if peer.Spec.Address == "" {
 			return fmt.Errorf("peer with index %d does not have the address defined", i)
diff --git a/internal/controller/wireguard_controller.go b/internal/controller/wireguard_controller.go
index 99dbc98..5215214 100644
--- a/internal/controller/wireguard_controller.go
+++ b/internal/controller/wireguard_controller.go
@@ -22,7 +22,6 @@ import (
 	"encoding/json"
 	"fmt"
 	"strconv"
-	"strings"
 	"time"
 
 	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
@@ -43,13 +42,6 @@ import (
 	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
 )
 
-const (
-	// defaultPersistentKeepalive is the fallback keepalive interval, in seconds, used when
-	// Wireguard.Spec.PersistentKeepalive is nil. The value users actually receive comes from
-	// the CRD default; this only covers objects stored before the field existed.
-	defaultPersistentKeepalive = 25
-)
-
 // Standard condition types for Wireguard
 const (
 	ConditionReady       = "Ready"
@@ -183,25 +175,10 @@ func effectivePeerCIDR4(wg *v1alpha1.Wireguard) string {
 	return ipam.DefaultPeerCIDR4
 }
 
-// persistentKeepalive returns the keepalive interval to write into peer configs.
-// A nil value means the field was absent — either the object predates the field or
-// defaulting is disabled — so fall back to the default rather than to zero, which
-// WireGuard interprets as "disabled".
-func persistentKeepalive(wg *v1alpha1.Wireguard) int32 {
-	if wg.Spec.PersistentKeepalive == nil {
-		return defaultPersistentKeepalive
-	}
-	return *wg.Spec.PersistentKeepalive
-}
-
-func (r *WireguardReconciler) updateWireguardPeers(ctx context.Context, req ctrl.Request, wireguard *v1alpha1.Wireguard, serverAddress string, dns string, dnsSearchDomain string, serverPublicKey string, serverMtu string) error {
-	// Rendered once and interpolated into every config flavor below. Empty when the
-	// interval is 0, so the line is omitted rather than written as an explicit zero.
-	keepaliveLine := ""
-	if ka := persistentKeepalive(wireguard); ka > 0 {
-		keepaliveLine = fmt.Sprintf("PersistentKeepalive = %d\n", ka)
-	}
-
+// updateWireguardPeers allocates an address to every peer of this Wireguard that has a
+// publicKey but no address, and marks each such peer Ready. A peer without a publicKey gets no
+// address: it is marked Ready=False with reason PublicKeyMissing.
+func (r *WireguardReconciler) updateWireguardPeers(ctx context.Context, req ctrl.Request, wireguard *v1alpha1.Wireguard) error {
 	peers, err := r.getWireguardPeers(ctx, req)
 	if err != nil {
 		return err
@@ -210,35 +187,17 @@ func (r *WireguardReconciler) updateWireguardPeers(ctx context.Context, req ctrl
 	cidr4 := effectivePeerCIDR4(wireguard)
 	usedIps4 := r.ipAllocator.GetUsedIPs(cidr4, peers)
 
-	// Ensure or create the aggregated peer configs Secret
-	peerCfgSecretName := wireguard.Name + "-peer-configs"
-	peerCfgSecret := &corev1.Secret{}
-	if err := r.Get(ctx, types.NamespacedName{Name: peerCfgSecretName, Namespace: wireguard.Namespace}, peerCfgSecret); err != nil {
-		if errors.IsNotFound(err) {
-			peerCfgSecret = &corev1.Secret{
-				ObjectMeta: metav1.ObjectMeta{
-					Name:      peerCfgSecretName,
-					Namespace: wireguard.Namespace,
-					OwnerReferences: []metav1.OwnerReference{
-						{
-							APIVersion: wireguard.APIVersion,
-							Kind:       wireguard.Kind,
-							Name:       wireguard.Name,
-							UID:        wireguard.UID,
-						},
-					},
-				},
-				Type: corev1.SecretTypeOpaque,
-				Data: map[string][]byte{},
+	for _, peer := range peers.Items {
+		if peer.Spec.PublicKey == "" {
+			if err := r.markPeerPublicKeyMissing(ctx, &peer); err != nil {
+				if errors.IsConflict(err) {
+					return nil
+				}
+				return err
 			}
-		} else {
-			return err
+			continue
 		}
-	}
-	// Always rebuild data from scratch to prune removed peers
-	newPeerCfgData := map[string][]byte{}
 
-	for _, peer := range peers.Items {
 		if peer.Spec.Address == "" {
 			ip, err := getAvaialbleIp(cidr4, usedIps4)
 			if err != nil {
@@ -251,52 +210,11 @@ func (r *WireguardReconciler) updateWireguardPeers(ctx context.Context, req ctrl
 			usedIps4 = append(usedIps4, ip)
 		}
 
-		dnsConfiguration := dns
-
-		if dnsSearchDomain != "" {
-			dnsConfiguration = dns + ", " + dnsSearchDomain
-		}
-
-		allowIps := peer.Spec.AllowedIPs
-
-		if allowIps == "" {
-			allowIps = "0.0.0.0/0"
-		}
-
-		// Do not store shell-wrapped config in status anymore per upstream PR 212
-
-		// Build pure peer config and write to aggregated Secret <wireguard-name>-peer-configs
-		// Fetch peer private key for inclusion
-		peerPrivSecret := &corev1.Secret{}
-		if err := r.Get(ctx, types.NamespacedName{Name: peer.Spec.PrivateKey.SecretKeyRef.Name, Namespace: peer.Namespace}, peerPrivSecret); err == nil {
-			if v, ok := peerPrivSecret.Data[peer.Spec.PrivateKey.SecretKeyRef.Key]; ok {
-				addresses := []string{}
-				if peer.Spec.Address != "" {
-					addresses = append(addresses, peer.Spec.Address)
-				}
-				addressLine := strings.Join(addresses, ", ")
-
-				pureCfg := fmt.Sprintf(`[Interface]
-PrivateKey = %s
-Address = %s
-DNS = %s`, strings.TrimSpace(string(v)), addressLine, dnsConfiguration)
-				if serverMtu != "" {
-					pureCfg = pureCfg + "\nMTU = " + serverMtu
-				}
-				pureCfg += fmt.Sprintf(`
-
-[Peer]
-PublicKey = %s
-AllowedIPs = %s
-Endpoint = %s:%s
-%s`, serverPublicKey, allowIps, serverAddress, wireguard.Status.Port, keepaliveLine)
-				newPeerCfgData[peer.Name] = []byte(pureCfg)
-			}
-		}
-		if peer.Status.Status != v1alpha1.Ready {
-			// Config removed from status; configs are stored in Secret <wg-name>-peer-configs
+		// A peer that was keyless still carries Ready=False/PublicKeyMissing; clear it.
+		if peer.Status.Status != v1alpha1.Ready || meta.FindStatusCondition(peer.Status.Conditions, ConditionReady) != nil {
 			peer.Status.Status = v1alpha1.Ready
 			peer.Status.Message = "Peer configured"
+			meta.RemoveStatusCondition(&peer.Status.Conditions, ConditionReady)
 			if err := r.Status().Update(ctx, &peer); err != nil {
 				if errors.IsConflict(err) {
 					return nil
@@ -306,29 +224,29 @@ Endpoint = %s:%s
 		}
 	}
 
-	// Assign rebuilt data map so removed peers are pruned
-	peerCfgSecret.Data = newPeerCfgData
-
-	// Create or update the aggregated peer configs Secret
-	existing := &corev1.Secret{}
-	if err := r.Get(ctx, types.NamespacedName{Name: peerCfgSecret.Name, Namespace: peerCfgSecret.Namespace}, existing); err != nil {
-		if errors.IsNotFound(err) {
-			if err := r.Create(ctx, peerCfgSecret); err != nil {
-				return err
-			}
-		} else {
-			return err
-		}
-	} else {
-		existing.Data = peerCfgSecret.Data
-		if err := r.Update(ctx, existing); err != nil {
-			return err
-		}
-	}
-
 	return nil
 }
 
+// markPeerPublicKeyMissing records that peer cannot be admitted by the server until it carries a
+// publicKey. It writes only when the recorded status differs, so repeated reconciles are no-ops.
+func (r *WireguardReconciler) markPeerPublicKeyMissing(ctx context.Context, peer *v1alpha1.WireguardPeer) error {
+	const reason = "PublicKeyMissing"
+	if c := meta.FindStatusCondition(peer.Status.Conditions, ConditionReady); c != nil && c.Reason == reason &&
+		c.Status == metav1.ConditionFalse && peer.Status.Status == v1alpha1.Pending {
+		return nil
+	}
+	peer.Status.Status = v1alpha1.Pending
+	peer.Status.Message = "Waiting for spec.publicKey"
+	meta.SetStatusCondition(&peer.Status.Conditions, metav1.Condition{
+		Type:               ConditionReady,
+		Status:             metav1.ConditionFalse,
+		Reason:             reason,
+		Message:            "spec.publicKey is empty, so the server cannot admit this peer",
+		ObservedGeneration: peer.Generation,
+	})
+	return r.Status().Update(ctx, peer)
+}
+
 //+kubebuilder:rbac:groups=vpn.wireguard-operator.io,resources=wireguards,verbs=get;list;watch;create;update;patch;delete
 //+kubebuilder:rbac:groups=vpn.wireguard-operator.io,resources=wireguards/status,verbs=get;update;patch
 //+kubebuilder:rbac:groups=vpn.wireguard-operator.io,resources=wireguards/finalizers,verbs=update
@@ -418,31 +336,6 @@ func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (
 		log.Info("Service annotations set without address; the LB controller must assign an IP", "wireguard", wireguard.Name, "namespace", wireguard.Namespace)
 	}
 
-	dnsAddress := "1.1.1.1"
-	dnsSearchDomain := ""
-
-	if wireguard.Spec.Dns != "" {
-		dnsAddress = wireguard.Spec.Dns
-	} else {
-		kubeDnsService := &corev1.Service{}
-		err = r.Get(ctx, types.NamespacedName{Name: "kube-dns", Namespace: "kube-system"}, kubeDnsService)
-		if err == nil {
-			dnsAddress = kubeDnsService.Spec.ClusterIP
-			dnsSearchDomain = fmt.Sprintf("%s.svc.cluster.local", wireguard.Namespace)
-		} else {
-			if errors.IsNotFound(err) {
-				log.Info("kube-dns service not found; defaulting DNS to public resolver", "dns", dnsAddress)
-			} else {
-				log.Error(err, "Unable to get kube-dns service; defaulting DNS to public resolver", "dns", dnsAddress)
-			}
-		}
-	}
-
-	// Allow overriding DNS search domain via spec
-	if wireguard.Spec.DnsSearchDomain != "" {
-		dnsSearchDomain = wireguard.Spec.DnsSearchDomain
-	}
-
 	// service
 	//
 	// Every field the builder renders converges: annotations, target ports, protocols and the
@@ -526,17 +419,16 @@ func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (
 		}
 	}
 
-	if wireguard.Status.Address != address || port != wireguard.Status.Port || dnsAddress != wireguard.Status.Dns {
+	if wireguard.Status.Address != address || port != wireguard.Status.Port {
 		latest := &v1alpha1.Wireguard{}
 		if err := r.Get(ctx, req.NamespacedName, latest); err == nil {
 			latest.Status.Address = address
 			latest.Status.Port = port
-			latest.Status.Dns = dnsAddress
 			if err := r.Status().Update(ctx, latest); err != nil {
 				if errors.IsNotFound(err) || errors.IsConflict(err) {
 					// ignore and let reconcile retry
 				} else {
-					log.Error(err, "Failed to update wireguard manifest address, port, and dns")
+					log.Error(err, "Failed to update wireguard manifest address and port")
 					return ctrl.Result{}, err
 				}
 			}
@@ -770,7 +662,7 @@ func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (
 		}
 	}
 
-	if err := r.updateWireguardPeers(ctx, req, wireguard, address, dnsAddress, dnsSearchDomain, string(secret.Data["publicKey"]), wireguard.Spec.Mtu); err != nil {
+	if err := r.updateWireguardPeers(ctx, req, wireguard); err != nil {
 		return ctrl.Result{}, err
 	}
 
diff --git a/internal/controller/wireguardpeer_controller.go b/internal/controller/wireguardpeer_controller.go
index b9cb8cf..4396826 100644
--- a/internal/controller/wireguardpeer_controller.go
+++ b/internal/controller/wireguardpeer_controller.go
@@ -21,10 +21,7 @@ import (
 	"fmt"
 
 	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
-	"github.com/jacaudi/wireguard-operator/internal/resources"
 
-	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"
-	corev1 "k8s.io/api/core/v1"
 	"k8s.io/apimachinery/pkg/api/errors"
 	"k8s.io/apimachinery/pkg/runtime"
 	"k8s.io/apimachinery/pkg/types"
@@ -38,8 +35,6 @@ import (
 type WireguardPeerReconciler struct {
 	client.Client
 	Scheme *runtime.Scheme
-
-	secretBuilder *resources.SecretBuilder
 }
 
 func (r *WireguardPeerReconciler) updateStatus(ctx context.Context, peer *v1alpha1.WireguardPeer, status string, message string) error {
@@ -86,12 +81,6 @@ func (r *WireguardPeerReconciler) Reconcile(ctx context.Context, req ctrl.Reques
 		return ctrl.Result{}, err
 	}
 
-	key, err := wgtypes.GeneratePrivateKey()
-	if err != nil {
-		log.Error(err, "Failed to generate private key")
-		return ctrl.Result{}, err
-	}
-
 	newPeer := peer.DeepCopy()
 	if newPeer.Status.Status == "" {
 		err = r.updateStatus(ctx, newPeer, v1alpha1.Pending, "Waiting for wireguard peer to be created")
@@ -103,55 +92,6 @@ func (r *WireguardPeerReconciler) Reconcile(ctx context.Context, req ctrl.Reques
 		return ctrl.Result{Requeue: true}, nil
 	}
 
-	if peer.Spec.PublicKey == "" {
-		secretName := peer.Name + "-peer"
-		existingSecret := &corev1.Secret{}
-		var publicKey string
-
-		err = r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: peer.Namespace}, existingSecret)
-		if err == nil {
-			// Secret already exists (previous attempt created it but the spec
-			// update was lost due to a conflict). Reuse the existing keys.
-			log.Info("Secret already exists, reusing existing keys", "secret.Name", secretName)
-			publicKey = string(existingSecret.Data["publicKey"])
-		} else if errors.IsNotFound(err) {
-			// Secret does not exist yet — generate new keys and create it.
-			publicKey = key.PublicKey().String()
-
-			secret, buildErr := r.secretBuilder.ForPeer(peer, key.String(), publicKey)
-			if buildErr != nil {
-				log.Error(buildErr, "Failed to build peer secret", "secret.Name", secretName)
-				return ctrl.Result{}, buildErr
-			}
-			log.Info("Creating a new secret", "secret.Namespace", secret.Namespace, "secret.Name", secret.Name)
-			if err = r.Create(ctx, secret); err != nil {
-				log.Error(err, "Failed to create new secret", "secret.Namespace", secret.Namespace, "secret.Name", secret.Name)
-				return ctrl.Result{}, err
-			}
-		} else {
-			log.Error(err, "Failed to check for existing secret", "secret.Name", secretName)
-			return ctrl.Result{}, err
-		}
-
-		// Use a merge patch to set only the key fields. Unlike Update(),
-		// a patch doesn't send the full object and won't conflict with
-		// concurrent updates to other fields (e.g. address allocation by
-		// the Wireguard controller).
-		patchBase := client.MergeFrom(peer.DeepCopy())
-		peer.Spec.PublicKey = publicKey
-		peer.Spec.PrivateKey = v1alpha1.PrivateKey{
-			SecretKeyRef: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secretName}, Key: "privateKey"}}
-		err = r.Patch(ctx, peer, patchBase)
-
-		if err != nil {
-			log.Error(err, "Failed to update peer with keys", "peer.Name", peer.Name)
-			return ctrl.Result{}, err
-		}
-
-		return ctrl.Result{Requeue: true}, nil
-
-	}
-
 	wireguard := &v1alpha1.Wireguard{}
 	err = r.Get(ctx, types.NamespacedName{Name: newPeer.Spec.WireguardRef, Namespace: newPeer.Namespace}, wireguard)
 
@@ -194,9 +134,6 @@ func (r *WireguardPeerReconciler) Reconcile(ctx context.Context, req ctrl.Reques
 		return ctrl.Result{}, nil
 	}
 
-	wireguardSecret := &corev1.Secret{}
-	_ = r.Get(ctx, types.NamespacedName{Name: newPeer.Spec.WireguardRef, Namespace: newPeer.Namespace}, wireguardSecret)
-
 	if len(newPeer.OwnerReferences) == 0 {
 		log.Info("Waiting for owner reference to be set " + wireguard.Name + " " + newPeer.Name)
 		patchBase := client.MergeFrom(newPeer.DeepCopy())
@@ -213,8 +150,6 @@ func (r *WireguardPeerReconciler) Reconcile(ctx context.Context, req ctrl.Reques
 		return ctrl.Result{Requeue: true}, nil
 	}
 
-	// No longer wait for a config in status; peer configs are stored in a Secret
-
 	return ctrl.Result{}, nil
 }
 
@@ -243,8 +178,6 @@ func (r *WireguardPeerReconciler) checkDuplicateAddress(ctx context.Context, nam
 
 // SetupWithManager sets up the controller with the Manager.
 func (r *WireguardPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
-	r.secretBuilder = resources.NewSecretBuilder(r.Scheme)
-
 	return ctrl.NewControllerManagedBy(mgr).
 		For(&v1alpha1.WireguardPeer{}).
 		Complete(r)
diff --git a/internal/resources/secret.go b/internal/resources/secret.go
index 2d7cc7c..101b82e 100644
--- a/internal/resources/secret.go
+++ b/internal/resources/secret.go
@@ -54,24 +54,3 @@ func (b *SecretBuilder) ForWireguard(wg *v1alpha1.Wireguard, state []byte, priva
 
 	return secret, nil
 }
-
-// ForPeer creates a secret for a WireguardPeer containing keys.
-func (b *SecretBuilder) ForPeer(peer *v1alpha1.WireguardPeer, privateKey, publicKey string) (*corev1.Secret, error) {
-	secret := &corev1.Secret{
-		ObjectMeta: metav1.ObjectMeta{
-			Name:      peer.Name + "-peer",
-			Namespace: peer.Namespace,
-			Labels:    LabelsForWireguard(peer.Name),
-		},
-		Data: map[string][]byte{
-			"privateKey": []byte(privateKey),
-			"publicKey":  []byte(publicKey),
-		},
-	}
-
-	if err := SetOwnerReference(peer, secret, b.scheme); err != nil {
-		return nil, err
-	}
-
-	return secret, nil
-}
~~~~

- [ ] **Step 6: Regenerate**

```bash
make manifests generate &&
AGENT=$(sed -n 's/^[[:space:]]*- --agent-image=//p' release.yaml | head -1) &&
MGR=$(sed -n 's/^        image: //p' release.yaml | head -1) &&
make generate-release-file AGENT_IMAGE="$AGENT" MANAGER_IMAGE="$MGR" > /dev/null &&
echo regen-ok
```

Expected: the block's last line is `regen-ok`. Anything else (a controller-gen or kustomize error) is `PLAN: BLOCKED — every path is a guess: regenerate`.

- [ ] **Step 7: Both new tests pass**

Run: `go test ./internal/agent/ -run TestIsStateValid -count=1 -v`
Expected: both PASS.

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -ginkgo.focus="writes no client config"`
Expected: `ok  	github.com/jacaudi/wireguard-operator/internal/controller`.

- [ ] **Step 8: Build and vet every build tag**

Run: `go build ./... && go vet ./... && go vet -tags=integration ./... && go vet -tags=e2e ./...`
Expected: no output, exit 0. A compile error here means a patch step was skipped or reordered: use the restart in Global Constraints.

- [ ] **Step 9: Unit tests**

Run: `go test ./... -race -count=1`
Expected: no `FAIL` and no `[FAIL]` anywhere in the output. Raw output shows `ok` per tested package; under the RTK hook it shows one `Go test: <n> passed in <m> packages` line.

- [ ] **Step 10: Integration suite (envtest)**

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race`
Expected: `ok  	github.com/jacaudi/wireguard-operator/internal/controller` after roughly 90 s; no `FAIL`. Known timing flake: if the only failure is the spec `no update churn applies each owned object at most once per reconcile and persists nothing` with the message `a reconcile was still in flight`, rerun the suite once. It failed 1 run in about 15 during this plan's verification, passed 3/3 on rerun of the same commit, and did not reproduce in the plan gate's 23 runs. A second failure, or any other failing spec, is real.

- [ ] **Step 11: Lint**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...`
Expected: the last line is exactly `0 issues.` Gate on that line, never on the exit status (the RTK hook swallows it).

- [ ] **Step 12: Format check (CI's `gofmt / goimports check`)**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint fmt --diff; echo fmt-exit=$?`
Expected: only `fmt-exit=0`. Any diff printed above it fails CI's required `ci` check even when lint says `0 issues.` (the lint ratchet does not see formatting); run `gofmt -w` on the files it names, rerun, and keep the result in this task's commit.

- [ ] **Step 13: YAML lint**

Run: `yamllint .`
Expected: no line containing `error`. One pre-existing `warning` for `./taskfile.yml` 52:3 is normal.

- [ ] **Step 14: Commit**

```bash
git add -A -- api config examples internal release.yaml
git commit -m "refactor!: delete peer config rendering and peer key minting" -m "Clients mint their own key in the pod and register only the public half (design 3.4), so the operator no longer writes <name>-peer-configs, <peer>-peer, DNS, MTU or keepalive. A peer without a publicKey gets no address and reports Ready=False, reason PublicKeyMissing. The agent no longer requires status.address or status.dns." -m "BREAKING CHANGE: spec.mtu, spec.dns, spec.dnsSearchDomain, spec.persistentKeepalive, status.dns and WireguardPeer spec.privateKeyRef, allowedIPs, dns and dnsSearchDomain are removed; the \`<name>-peer-configs\` and \`<peer>-peer\` Secrets are no longer written; peers must supply spec.publicKey."
```

- [ ] **Step 15: Confirm the tree is clean and the commit is signed**

Run: `git status --porcelain`
Expected: no output. If anything is listed, it is a path this task touched but the `git add` above missed: `git add` it, `git commit --amend --no-edit`, rerun this step. A second non-empty result is `PLAN: BLOCKED — every path is a guess: dirty tree after commit`.

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. Any other value, a signing error, or a hang at the commit step is `PLAN: BLOCKED` (never `--no-gpg-sign`).

- [ ] **Step 16: Generated files and release.yaml still match their sources**

Run: `make -s manifests generate && git status --porcelain`
Expected: no output. If files are listed, the regenerate block (Task 2 Step 5) was skipped or an API edit came after it: `git add -A -- api config && git commit --amend --no-edit`, rerun. A second failure is `PLAN: BLOCKED — every path is a guess: codegen drift`.

Run: `sh hack/release-file-drift.sh`
Expected: the last line is `release.yaml matches config/`. If not, rerun the regenerate block (Task 2 Step 5 shows it), `git add release.yaml && git commit --amend --no-edit`, rerun this check. A second failure is `PLAN: BLOCKED — every path is a guess: release.yaml drift`.

---

### Task 6: Make the server Service always ClusterIP

Kind: refactor

**Files:**
- Create: `internal/resources/service_test.go`
- Delete: `examples/serverWithNodePortService.yaml`
- Modify:
  - `api/v1alpha1/wireguard_types.go`: `Address`, `ExternalAddress`, `ServiceType`, `NodePort` (json `port`) and `ServiceAnnotations`; `Status.Address`, `Status.Port`, `Status.UniqueIdentifier` and `Status.Resources`; the `Resource` type; the `Address` print column.
  - `internal/resources/service.go`.
  - `internal/controller/wireguard_controller.go`: `getNodeIps`, the LoadBalancer and NodePort branches, the status address/port write, the resource-status block, and the `nodes` RBAC marker.
  - `internal/controller/wireguard_controller_test.go`, `internal/controller/churn_test.go` and `internal/it/it_test.go`.
- Regenerated: deepcopy, `vpn.wireguard-operator.io_wireguards.yaml`, `config/rbac/role.yaml` (drops `nodes`), `release.yaml`

**Interfaces:**
- Consumes: Task 2's `testScheme`/`testWireguard` helpers; Task 5's controller.
- Produces: `func (b *ServiceBuilder) ForWireguard(wg *v1alpha1.Wireguard) (*corev1.Service, error)`. It always returns `Type: ClusterIP` with one port, `wireguard`/UDP/51820, and no annotations or nodePort. A reconcile no longer stops early to write `status.address`/`status.port`.

- [ ] **Step 1: Remove the LoadBalancer/NodePort helpers and specs**

```bash
perl -i -ne 'print unless /^func createNode\(address string\) error \{$/ .. /^\}$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^func reconcileServiceWithTypeNodePort\(/ .. /^\}$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\/\/ reconcileServiceWithTypeLoadBalancer asserts internally/ .. /^\}$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^func reconcileServiceWithClusterIP\(/ .. /^\}$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t\tIt\("Should create a WG with ServiceType NodePort and WG peer successfully", func\(\) \{$/ .. /^\t\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t\tIt\("Should create a WG with ServiceType LoadBalancer and WG peer successfully", func\(\) \{$/ .. /^\t\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t\tIt\("does not set loadBalancerIP when annotations are provided without address", func\(\) \{$/ .. /^\t\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t\tIt\("removes a service annotation the user has deleted from the spec", func\(\) \{$/ .. /^\t\t\}\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t+Expect\(reconcileServiceWithClusterIP\(serviceKey, 51820\)\)\.Should\(Succeed\(\)\)$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t+\/\/ See reconcileServiceWithTypeLoadBalancer: wait for the Service, then call it once\.$/' internal/controller/wireguard_controller_test.go
perl -i -ne 'print unless /^\t+Expect\(reconcileServiceWithTypeLoadBalancer\(serviceKey, "localhost"\)\)\.Should\(Succeed\(\)\)$/' internal/controller/wireguard_controller_test.go
gofmt -w internal/controller/wireguard_controller_test.go
```

Run: `cat internal/controller/wireguard_controller_test.go | grep -cF -e 'func createNode' -e 'func reconcileServiceWithTypeNodePort' -e 'func reconcileServiceWithTypeLoadBalancer' -e 'func reconcileServiceWithClusterIP' -e 'ServiceType NodePort and WG peer' -e 'ServiceType LoadBalancer and WG peer' -e 'does not set loadBalancerIP' -e 'removes a service annotation' -e 'reconcileServiceWithClusterIP(' -e 'reconcileServiceWithTypeLoadBalancer('`
Expected: `0`. A non-zero count means an anchor did not match (the file is not in the state this plan assumes): use the restart in Global Constraints; if it recurs, `PLAN: BLOCKED — every path is a guess: plan drift <task>/<step>`.

- [ ] **Step 2: Adapt the remaining specs and add the leftover-LoadBalancer spec (Review Focus 3)**

```bash
awk -v id=t6-test '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t6-test -->
~~~~diff
diff --git a/internal/controller/churn_test.go b/internal/controller/churn_test.go
index c985369..68724ba 100644
--- a/internal/controller/churn_test.go
+++ b/internal/controller/churn_test.go
@@ -117,7 +117,6 @@ var _ = Describe("no update churn", func() {
 		wg := &v1alpha1.Wireguard{
 			ObjectMeta: metav1.ObjectMeta{Name: churnName, Namespace: churnNS},
 		}
-		wg.Spec.ServiceType = corev1.ServiceTypeClusterIP
 		Expect(k8sClient.Create(context.Background(), wg)).Should(Succeed())
 		defer func() {
 			_ = k8sClient.Delete(context.Background(), wg)
diff --git a/internal/controller/wireguard_controller_test.go b/internal/controller/wireguard_controller_test.go
index 824ee72..1eb4d9b 100644
--- a/internal/controller/wireguard_controller_test.go
+++ b/internal/controller/wireguard_controller_test.go
@@ -6,7 +6,6 @@ import (
 	"context"
 	"fmt"
 	"slices"
-	"strconv"
 	"time"
 
 	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
@@ -21,12 +20,9 @@ import (
 	"k8s.io/apimachinery/pkg/api/resource"
 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 	"k8s.io/apimachinery/pkg/types"
-	"k8s.io/apimachinery/pkg/util/intstr"
 	"sigs.k8s.io/controller-runtime/pkg/client"
 )
 
-// test helpers
-
 var _ = Describe("wireguard controller", func() {
 
 	// Define utility constants for object names and testing timeouts/durations and intervals.
@@ -90,7 +86,6 @@ var _ = Describe("wireguard controller", func() {
 		It("allocates an address to a publicKey peer and writes no client config", func() {
 			wgServer := &v1alpha1.Wireguard{
 				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
-				Spec:       v1alpha1.WireguardSpec{ServiceType: corev1.ServiceTypeClusterIP},
 			}
 			Expect(k8sClient.Create(context.Background(), wgServer)).Should(Succeed())
 
@@ -189,9 +184,7 @@ var _ = Describe("wireguard controller", func() {
 			}, Timeout, Interval).Should(Equal(v1alpha1.Ready), "a peer that gains a key is admitted and its PublicKeyMissing condition cleared")
 		})
 
-		It("Should create a WG with ServiceType ClusterIP and WG peer successfully", func() {
-			expectedAddress := "test-address"
-
+		It("Should create a WG with a ClusterIP Service and WG peer successfully", func() {
 			wgKey := types.NamespacedName{
 				Name:      wgName,
 				Namespace: wgNamespace,
@@ -201,10 +194,6 @@ var _ = Describe("wireguard controller", func() {
 					Name:      wgKey.Name,
 					Namespace: wgKey.Namespace,
 				},
-				Spec: v1alpha1.WireguardSpec{
-					ServiceType: corev1.ServiceTypeClusterIP,
-					Address:     expectedAddress,
-				},
 			}
 			expectedLabels := map[string]string{"app": "wireguard", "instance": wgKey.Name}
 
@@ -248,10 +237,10 @@ var _ = Describe("wireguard controller", func() {
 			Eventually(func() []string {
 				err := k8sClient.Get(context.Background(), wgKey, wg)
 				if err != nil {
-					return []string{"", "", "", "", ""}
+					return []string{"", ""}
 				}
-				return []string{wg.Status.Address, wg.Status.Port, wg.Status.Status, wg.Status.Message}
-			}, Timeout, Interval).Should(Equal([]string{expectedAddress, "51820", "ready", "VPN is active!"}))
+				return []string{wg.Status.Status, wg.Status.Message}
+			}, Timeout, Interval).Should(Equal([]string{"ready", "VPN is active!"}))
 
 			Eventually(func() string {
 				deploymentKey := types.NamespacedName{
@@ -329,9 +318,7 @@ var _ = Describe("wireguard controller", func() {
 					Namespace: wgKey.Namespace,
 				},
 				Spec: v1alpha1.WireguardSpec{
-					ServiceType: corev1.ServiceTypeClusterIP,
-					Address:     "test-address",
-					PeerCIDR:    customCIDR,
+					PeerCIDR: customCIDR,
 				},
 			}
 			expectedLabels := map[string]string{"app": "wireguard", "instance": wgKey.Name}
@@ -397,10 +384,6 @@ var _ = Describe("wireguard controller", func() {
 					Name:      wgKey.Name,
 					Namespace: wgKey.Namespace,
 				},
-				Spec: v1alpha1.WireguardSpec{
-					ServiceType: corev1.ServiceTypeClusterIP,
-					Address:     "test-address",
-				},
 			}
 			Expect(k8sClient.Create(context.Background(), wgServer)).Should(Succeed())
 
@@ -518,10 +501,6 @@ var _ = Describe("wireguard controller", func() {
 					Name:      wgKey.Name,
 					Namespace: wgKey.Namespace,
 				},
-				Spec: v1alpha1.WireguardSpec{
-					ServiceType: corev1.ServiceTypeClusterIP,
-					Address:     "test-address",
-				},
 			}
 			Expect(k8sClient.Create(context.Background(), wgServer)).Should(Succeed())
 
@@ -617,8 +596,6 @@ var _ = Describe("wireguard controller", func() {
 						Namespace: wgKey.Namespace,
 					},
 					Spec: v1alpha1.WireguardSpec{
-						ServiceType:                  corev1.ServiceTypeClusterIP,
-						Address:                      "test-address",
 						UseWgUserspaceImplementation: useWgUserspace,
 					},
 				}
@@ -707,11 +684,10 @@ var _ = Describe("wireguard controller", func() {
 			}, Timeout, Interval).Should(Equal(wgTestImage))
 		})
 
-		It("preserves the apiserver's clusterIP and nodePort allocations across reconciles", func() {
+		It("preserves the apiserver's clusterIP allocation across reconciles", func() {
 			created := &v1alpha1.Wireguard{
 				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
 			}
-			created.Spec.ServiceType = corev1.ServiceTypeNodePort
 			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())
 
 			serviceKey := types.NamespacedName{Name: wgName + "-svc", Namespace: wgNamespace}
@@ -720,8 +696,8 @@ var _ = Describe("wireguard controller", func() {
 				if err := k8sClient.Get(context.Background(), serviceKey, svc); err != nil {
 					return false
 				}
-				return len(svc.Spec.Ports) > 0 && svc.Spec.Ports[0].NodePort != 0 && svc.Spec.ClusterIP != ""
-			}, Timeout, Interval).Should(BeTrue(), "apiserver should have allocated clusterIP and nodePort")
+				return svc.Spec.ClusterIP != ""
+			}, Timeout, Interval).Should(BeTrue(), "apiserver should have allocated a clusterIP")
 
 			first := &corev1.Service{}
 			Expect(k8sClient.Get(context.Background(), serviceKey, first)).Should(Succeed())
@@ -746,10 +722,8 @@ var _ = Describe("wireguard controller", func() {
 				if err := k8sClient.Get(context.Background(), serviceKey, svc); err != nil {
 					return false
 				}
-				return svc.Spec.ClusterIP == first.Spec.ClusterIP &&
-					len(svc.Spec.Ports) > 0 &&
-					svc.Spec.Ports[0].NodePort == first.Spec.Ports[0].NodePort
-			}, "3s", Interval).Should(BeTrue(), "server-assigned clusterIP/nodePort must survive apply")
+				return svc.Spec.ClusterIP == first.Spec.ClusterIP
+			}, "3s", Interval).Should(BeTrue(), "the server-assigned clusterIP must survive apply")
 		})
 
 		It("adopts a Deployment written before Server-Side Apply, so a later removal converges", func() {
@@ -840,18 +814,17 @@ var _ = Describe("wireguard controller", func() {
 			Expect(managers).ShouldNot(ContainElement("manager"), "the legacy Update entry must be gone")
 		})
 
-		It("adopts a legacy Service without moving its clusterIP or nodePort allocation", func() {
+		It("adopts a legacy Service without moving its clusterIP allocation", func() {
 			// Spec 6's Service variant (design §10.2 item 10): if the legacy entry owned an
 			// allocated field, migration would hand it to the operator, whose next apply
-			// relinquishes it. The assertions below pin that it owns neither clusterIP nor
-			// nodePort, and that both allocations survive adoption.
+			// relinquishes it. The assertions below pin that it does not own clusterIP, and
+			// that the allocation survives adoption.
 			// Same seeding rules as above; BeforeEach already deletes Services.
 			serviceKey := types.NamespacedName{Name: wgName + "-svc", Namespace: wgNamespace}
 			owner := &v1alpha1.Wireguard{
 				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
 			}
-			owner.Spec.ServiceType = corev1.ServiceTypeNodePort
-			legacy, err := resources.NewServiceBuilder(k8sClient.Scheme()).ForWireguard(owner, corev1.ServiceTypeNodePort)
+			legacy, err := resources.NewServiceBuilder(k8sClient.Scheme()).ForWireguard(owner)
 			Expect(err).ShouldNot(HaveOccurred())
 			legacy.OwnerReferences = nil
 			Expect(k8sClient.Create(context.Background(), legacy, client.FieldOwner("manager"))).Should(Succeed())
@@ -859,11 +832,9 @@ var _ = Describe("wireguard controller", func() {
 			seeded := &corev1.Service{}
 			Expect(k8sClient.Get(context.Background(), serviceKey, seeded)).Should(Succeed())
 			Expect(seeded.Spec.ClusterIP).ShouldNot(BeEmpty())
-			Expect(seeded.Spec.Ports).ShouldNot(BeEmpty())
-			Expect(seeded.Spec.Ports[0].NodePort).ShouldNot(BeZero())
 
-			// The apiserver allocated clusterIP and nodePort itself, so the legacy Update entry
-			// must not own them: there is nothing allocated for the migration to hand over.
+			// The apiserver allocated clusterIP itself, so the legacy Update entry must not own
+			// it: there is nothing allocated for the migration to hand over.
 			var legacyFields string
 			for _, mf := range seeded.ManagedFields {
 				if mf.Manager == "manager" && mf.Operation == metav1.ManagedFieldsOperationUpdate && mf.FieldsV1 != nil {
@@ -872,7 +843,6 @@ var _ = Describe("wireguard controller", func() {
 			}
 			Expect(legacyFields).ShouldNot(BeEmpty(), "the seed must carry an Update entry for manager")
 			Expect(legacyFields).ShouldNot(ContainSubstring(`"f:clusterIP"`), "the legacy entry must not own clusterIP")
-			Expect(legacyFields).ShouldNot(ContainSubstring(`"f:nodePort"`), "the legacy entry must not own nodePort")
 
 			Expect(k8sClient.Create(context.Background(), owner)).Should(Succeed())
 
@@ -890,17 +860,56 @@ var _ = Describe("wireguard controller", func() {
 			}, Timeout, Interval).Should(BeTrue(), "the operator should adopt the legacy Service")
 
 			Expect(adopted.Spec.ClusterIP).Should(Equal(seeded.Spec.ClusterIP))
-			Expect(adopted.Spec.Ports).ShouldNot(BeEmpty())
-			Expect(adopted.Spec.Ports[0].NodePort).Should(Equal(seeded.Spec.Ports[0].NodePort))
+		})
+
+		It("converts a Service left as LoadBalancer by an earlier operator to ClusterIP", func() {
+			// A cluster that ran the operator before the trim can still hold <name>-svc as a
+			// LoadBalancer written by the "manager" field manager. It must converge, not wedge.
+			serviceKey := types.NamespacedName{Name: wgName + "-svc", Namespace: wgNamespace}
+			labels := resources.LabelsForWireguard(wgName)
+			legacy := &corev1.Service{
+				ObjectMeta: metav1.ObjectMeta{Name: serviceKey.Name, Namespace: serviceKey.Namespace, Labels: labels},
+				Spec: corev1.ServiceSpec{
+					Type:     corev1.ServiceTypeLoadBalancer,
+					Selector: labels,
+					Ports: []corev1.ServicePort{{
+						Name: "wireguard", Protocol: corev1.ProtocolUDP, Port: resources.WireguardPort,
+					}},
+				},
+			}
+			Expect(k8sClient.Create(context.Background(), legacy, client.FieldOwner("manager"))).Should(Succeed())
+
+			created := &v1alpha1.Wireguard{
+				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
+			}
+			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())
+
+			Eventually(func() corev1.ServiceType {
+				svc := &corev1.Service{}
+				if err := k8sClient.Get(context.Background(), serviceKey, svc); err != nil {
+					return ""
+				}
+				return svc.Spec.Type
+			}, Timeout, Interval).Should(Equal(corev1.ServiceTypeClusterIP))
+
+			Eventually(func() string {
+				wg := &v1alpha1.Wireguard{}
+				if err := k8sClient.Get(context.Background(), wgKey, wg); err != nil {
+					return ""
+				}
+				return wg.Status.Status
+			}, Timeout, Interval).Should(Equal(v1alpha1.Ready))
 		})
 
 		It("reports Degraded while an apply can never succeed, and clears it once one does", func() {
-			// nodePort on a ClusterIP Service is rejected by the apiserver, permanently.
+			// A memory request above its limit is rejected by the apiserver, permanently.
 			created := &v1alpha1.Wireguard{
 				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
 			}
-			created.Spec.ServiceType = corev1.ServiceTypeClusterIP
-			created.Spec.NodePort = 30123
+			created.Spec.Agent.Resources = corev1.ResourceRequirements{
+				Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("256Mi")},
+				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")},
+			}
 			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())
 
 			Eventually(func() string {
@@ -914,7 +923,7 @@ var _ = Describe("wireguard controller", func() {
 					}
 				}
 				return "no-degraded-condition"
-			}, Timeout, Interval).Should(Equal("ServiceApplyFailed"))
+			}, Timeout, Interval).Should(Equal("DeploymentApplyFailed"))
 
 			// Remove the cause. The next reconcile applies every object, so Ready must be True and
 			// Degraded must be False -- not left True beside it from the earlier failure.
@@ -923,7 +932,7 @@ var _ = Describe("wireguard controller", func() {
 				if err := k8sClient.Get(context.Background(), wgKey, wg); err != nil {
 					return err
 				}
-				wg.Spec.NodePort = 0
+				wg.Spec.Agent.Resources = corev1.ResourceRequirements{}
 				return k8sClient.Update(context.Background(), wg)
 			}, Timeout, Interval).Should(Succeed())
 
diff --git a/internal/it/it_test.go b/internal/it/it_test.go
index c978781..8b25cae 100644
--- a/internal/it/it_test.go
+++ b/internal/it/it_test.go
@@ -15,8 +15,7 @@ var _ = Describe("wireguard controller", func() {
 kind: Wireguard
 metadata:
   name: vpn
-spec:
-  serviceType: "NodePort"`
+spec: {}`
 
 		// kubectl apply -f
 		output, err := KubectlApply(wireguardYaml, TestNamespace)
~~~~

- [ ] **Step 3: Guard: no `//nolint:errcheck` directive is left (Task 5 removed them; errcheck is excluded for tests and the ratchet flags them as unused)**

```bash
perl -i -ne 'print unless /^\t+\/\/nolint:errcheck$/' internal/controller/wireguard_controller_test.go
```

Run: `grep -cF '//nolint:errcheck' internal/controller/wireguard_controller_test.go`
Expected: `0`.

- [ ] **Step 4: Write the failing builder test**

```bash
awk -v id=t6-service-test '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md > internal/resources/service_test.go
```

<!-- patch t6-service-test -->
~~~~go
package resources

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestServiceIsClusterIPWithOnlyTheWireguardPort(t *testing.T) {
	svc, err := NewServiceBuilder(testScheme(t)).ForWireguard(testWireguard())
	if err != nil {
		t.Fatalf("ForWireguard: %v", err)
	}

	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("type = %q, want %q", svc.Spec.Type, corev1.ServiceTypeClusterIP)
	}
	if len(svc.Annotations) != 0 {
		t.Errorf("annotations = %v, want none", svc.Annotations)
	}
	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("ports = %v, want exactly one", svc.Spec.Ports)
	}
	p := svc.Spec.Ports[0]
	if p.Name != "wireguard" || p.Protocol != corev1.ProtocolUDP || p.Port != WireguardPort || p.NodePort != 0 {
		t.Errorf("port = %+v, want wireguard/UDP/%d with no nodePort", p, WireguardPort)
	}
}
~~~~

Run: `wc -l < internal/resources/service_test.go | tr -d ' '`
Expected: `28`. Anything else means the extraction picked up the wrong block: delete the file and rerun the step.

- [ ] **Step 5: Watch it fail**

Run: `go test ./internal/resources/ -run TestServiceIsClusterIPWithOnlyTheWireguardPort -count=1`
Expected: build failure `not enough arguments in call to NewServiceBuilder(testScheme(t)).ForWireguard` (the builder still takes a `serviceType`).

- [ ] **Step 6: Make the Service ClusterIP-only and delete the external-address plumbing**

```bash
awk -v id=t6-impl '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t6-impl -->
~~~~diff
diff --git a/api/v1alpha1/wireguard_types.go b/api/v1alpha1/wireguard_types.go
index 383d4e4..979eb3a 100644
--- a/api/v1alpha1/wireguard_types.go
+++ b/api/v1alpha1/wireguard_types.go
@@ -30,22 +30,9 @@ const (
 
 // WireguardSpec defines the desired state of Wireguard
 type WireguardSpec struct {
-	// A string field that specifies the address for the Wireguard Service.
-	// When ServiceType is LoadBalancer, this can be used to request a fixed Service IP.
-	// If ExternalAddress is not set, this value is also used as the peer endpoint.
-	Address string `json:"address,omitempty"`
-	// A string field that specifies the external address where peers should connect to this Wireguard instance.
-	// When set, this is used for peer endpoints instead of Address or Service status.
-	ExternalAddress string `json:"externalAddress,omitempty"`
 	// PeerCIDR is the IPv4 CIDR range from which Wireguard peer addresses will be allocated.
 	// If omitted, the operator will default to 10.8.0.0/24.
 	PeerCIDR string `json:"peerCIDR,omitempty"`
-	// A field that specifies the type of Kubernetes service that should be used for the Wireguard VPN. This could be ClusterIP, NodePort or LoadBalancer, depending on the needs of the deployment.
-	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
-	// A field that specifies the value to use for a nodePort ServiceType
-	NodePort int32 `json:"port,omitempty"`
-	// A map of key value strings for service annotations
-	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`
 	// A boolean field that specifies whether IP forwarding should be enabled on the Wireguard VPN pod at startup. This can be useful to enable if the peers are having problems with sending traffic to the internet.
 	EnableIpForwardOnPodInit bool `json:"enableIpForwardOnPodInit,omitempty"`
 	// A boolean field that specifies whether to use the userspace implementation of Wireguard instead of the kernel one.
@@ -63,18 +50,10 @@ type WireguardPodSpec struct {
 
 // WireguardStatus defines the observed state of Wireguard
 type WireguardStatus struct {
-	// A string field that specifies the address for the Wireguard VPN server that is currently being used.
-	Address string `json:"address,omitempty"`
-	// A string field that specifies the port for the Wireguard VPN server that is currently being used.
-	Port string `json:"port,omitempty"`
 	// A string field that represents the current status of Wireguard. This could include values like ready, pending, or error.
 	Status string `json:"status,omitempty"`
 	// A string field that provides additional information about the status of Wireguard. This could include error messages or other information that helps to diagnose issues with the wg instance.
 	Message string `json:"message,omitempty"`
-	// A stable identifier for the Wireguard instance (e.g., server public key).
-	UniqueIdentifier string `json:"uniqueIdentifier,omitempty"`
-	// Resource-level status information for associated Kubernetes resources.
-	Resources []Resource `json:"resources,omitempty"`
 	// Conditions represent the latest available observations of the Wireguard's state.
 	// +optional
 	// +patchMergeKey=type
@@ -84,22 +63,9 @@ type WireguardStatus struct {
 	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
 }
 
-// Resource describes the observed state of a related Kubernetes resource.
-type Resource struct {
-	// The resource name (e.g., my-wg-dep, my-wg-svc).
-	Name string `json:"name,omitempty"`
-	// The resource type (e.g., Deployment, Service, ConfigMap, Secret).
-	Type string `json:"type,omitempty"`
-	// A short status string (e.g., Ready, Pending, Error).
-	Status string `json:"status,omitempty"`
-	// The container image when applicable (e.g., for Deployments).
-	Image string `json:"image,omitempty"`
-}
-
 //+kubebuilder:object:root=true
 //+kubebuilder:subresource:status
 //+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
-//+kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.address`
 //+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
 
 // Wireguard is the Schema for the wireguards API
diff --git a/internal/controller/wireguard_controller.go b/internal/controller/wireguard_controller.go
index 1ee55db..129955e 100644
--- a/internal/controller/wireguard_controller.go
+++ b/internal/controller/wireguard_controller.go
@@ -20,8 +20,6 @@ import (
 	"bytes"
 	"context"
 	"encoding/json"
-	"fmt"
-	"strconv"
 	"time"
 
 	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
@@ -81,35 +79,6 @@ func (r *WireguardReconciler) getWireguardPeers(ctx context.Context, req ctrl.Re
 	return relatedPeers, nil
 }
 
-func (r *WireguardReconciler) getNodeIps(ctx context.Context) ([]string, error) {
-	nodes := &corev1.NodeList{}
-	if err := r.List(ctx, nodes); err != nil {
-		return nil, err
-	}
-
-	ips := []string{}
-
-	for _, node := range nodes.Items {
-		for _, address := range node.Status.Addresses {
-			if address.Type == corev1.NodeExternalIP {
-				ips = append(ips, address.Address)
-			}
-		}
-	}
-
-	if len(ips) == 0 {
-		for _, node := range nodes.Items {
-			for _, address := range node.Status.Addresses {
-				if address.Type == corev1.NodeInternalIP {
-					ips = append(ips, address.Address)
-				}
-			}
-		}
-	}
-
-	return ips, nil
-}
-
 func (r *WireguardReconciler) updateStatus(ctx context.Context, req ctrl.Request, cond metav1.Condition) error {
 	latest := &v1alpha1.Wireguard{}
 	if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
@@ -224,7 +193,6 @@ func (r *WireguardReconciler) updateWireguardPeers(ctx context.Context, req ctrl
 //+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
 //+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
 //+kubebuilder:rbac:groups="apps",resources=pods,verbs=get;list;watch;create;update;patch;delete
-//+kubebuilder:rbac:groups="",resources=nodes,verbs=list;watch
 //+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
 //+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
 
@@ -294,115 +262,17 @@ func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (
 		filteredPeers = append(filteredPeers, peer)
 	}
 
-	serviceType := corev1.ServiceTypeLoadBalancer
-
-	if wireguard.Spec.ServiceType != "" {
-		serviceType = wireguard.Spec.ServiceType
-	}
-	if serviceType == corev1.ServiceTypeLoadBalancer && len(wireguard.Spec.ServiceAnnotations) > 0 && wireguard.Spec.Address == "" {
-		log.Info("Service annotations set without address; the LB controller must assign an IP", "wireguard", wireguard.Name, "namespace", wireguard.Namespace)
-	}
-
 	// service
 	//
-	// Every field the builder renders converges: annotations, target ports, protocols and the
-	// second port in dualMode. Server-assigned fields survive because the builder never renders
-	// them: clusterIP and ipFamilies are absent from the applied body, and a nodePort of 0 is
-	// dropped by omitempty, so the operator never takes ownership of the apiserver's allocation.
-	desiredSvc, err := r.serviceBuilder.ForWireguard(wireguard, serviceType)
+	// Every field the builder renders converges. The server-assigned clusterIP survives because
+	// the builder never renders it, so the operator never takes ownership of the allocation.
+	desiredSvc, err := r.serviceBuilder.ForWireguard(wireguard)
 	if err != nil {
 		return r.failDegraded(ctx, req, err, "ServiceBuildFailed", "Failed to build service")
 	}
 	if err := r.applyOwned(ctx, desiredSvc); err != nil {
 		return r.failDegraded(ctx, req, err, "ServiceApplyFailed", "Failed to apply service")
 	}
-	svcFound := desiredSvc
-
-	address := wireguard.Spec.ExternalAddress
-	if address == "" {
-		address = wireguard.Spec.Address
-	}
-
-	var port = fmt.Sprintf("%d", resources.WireguardPort)
-
-	if serviceType == corev1.ServiceTypeLoadBalancer {
-		ingressList := svcFound.Status.LoadBalancer.Ingress
-		log.Info("Found ingress", "ingress", ingressList)
-		if len(ingressList) == 0 {
-			err = r.updateStatus(ctx, req, metav1.Condition{Type: ConditionProgressing, Status: metav1.ConditionTrue, Reason: "ServiceNotReady", Message: "Waiting for service to be ready"})
-			if err != nil {
-				return ctrl.Result{}, err
-			}
-
-			return ctrl.Result{}, nil
-		}
-
-		if address == "" {
-			address = svcFound.Status.LoadBalancer.Ingress[0].Hostname
-
-		}
-		if address == "" {
-			address = svcFound.Status.LoadBalancer.Ingress[0].IP
-		}
-	}
-	if serviceType == corev1.ServiceTypeNodePort {
-		if len(svcFound.Spec.Ports) == 0 {
-			err = r.updateStatus(ctx, req, metav1.Condition{Type: ConditionProgressing, Status: metav1.ConditionTrue, Reason: "NodePortPending", Message: "Waiting for service with type NodePort to be ready"})
-			if err != nil {
-				return ctrl.Result{}, err
-			}
-
-			return ctrl.Result{}, nil
-		}
-
-		port = strconv.Itoa(int(svcFound.Spec.Ports[0].NodePort))
-
-		ips, err := r.getNodeIps(ctx)
-
-		if err != nil {
-			return ctrl.Result{}, err
-		}
-		if address == "" {
-			if len(ips) == 0 {
-				err = r.updateStatus(ctx, req, metav1.Condition{Type: ConditionProgressing, Status: metav1.ConditionTrue, Reason: "AwaitingNodeIPs", Message: "Unable to determine WG address through nodes addresses. Please set Wireguard.Spec.ExternalAddress or Wireguard.Spec.Address if necessary."})
-				if err != nil {
-					return ctrl.Result{}, err
-				}
-				return ctrl.Result{}, nil
-			}
-			address = ips[0]
-		}
-
-	}
-
-	if serviceType == corev1.ServiceTypeClusterIP {
-		if len(svcFound.Spec.Ports) == 0 {
-			err = r.updateStatus(ctx, req, metav1.Condition{Type: ConditionProgressing, Status: metav1.ConditionTrue, Reason: "ClusterIPPending", Message: "Waiting for service with type ClusterIP to be ready"})
-			if err != nil {
-				return ctrl.Result{}, err
-			}
-
-			return ctrl.Result{}, nil
-		}
-	}
-
-	if wireguard.Status.Address != address || port != wireguard.Status.Port {
-		latest := &v1alpha1.Wireguard{}
-		if err := r.Get(ctx, req.NamespacedName, latest); err == nil {
-			latest.Status.Address = address
-			latest.Status.Port = port
-			if err := r.Status().Update(ctx, latest); err != nil {
-				if errors.IsNotFound(err) || errors.IsConflict(err) {
-					// ignore and let reconcile retry
-				} else {
-					log.Error(err, "Failed to update wireguard manifest address and port")
-					return ctrl.Result{}, err
-				}
-			}
-		}
-		// requeue to continue processing with updated status
-		return ctrl.Result{}, nil
-	}
 
 	// fetch secret
 	secret := &corev1.Secret{}
@@ -543,92 +413,6 @@ func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (
 		return r.failDegraded(ctx, req, err, "DeploymentApplyFailed", "Failed to apply deployment")
 	}
 
-	// Update resource-level status and unique identifier if available
-	{
-		resourcesStatus := make([]v1alpha1.Resource, 0, 4)
-
-		// Service status
-		svcStatus := "Pending"
-		switch serviceType {
-		case corev1.ServiceTypeLoadBalancer:
-			if len(svcFound.Status.LoadBalancer.Ingress) > 0 {
-				svcStatus = "Ready"
-			}
-		case corev1.ServiceTypeNodePort:
-			if len(svcFound.Spec.Ports) > 0 && svcFound.Spec.Ports[0].NodePort != 0 {
-				svcStatus = "Ready"
-			}
-		default: // ClusterIP
-			if svcFound.Spec.ClusterIP != "" {
-				svcStatus = "Ready"
-			}
-		}
-		resourcesStatus = append(resourcesStatus, v1alpha1.Resource{Name: svcFound.Name, Type: "Service", Status: svcStatus})
-
-		// ConfigMap status
-		cfgStatus := "Pending"
-		if configFound.Name != "" {
-			cfgStatus = "Ready"
-		}
-		resourcesStatus = append(resourcesStatus, v1alpha1.Resource{Name: configFound.Name, Type: "ConfigMap", Status: cfgStatus})
-
-		// Secret status and unique identifier
-		secStatus := "Pending"
-		uniqueIdentifier := ""
-		if secret.Name != "" {
-			secStatus = "Ready"
-			uniqueIdentifier = string(secret.Data["publicKey"])
-		}
-		resourcesStatus = append(resourcesStatus, v1alpha1.Resource{Name: wireguard.Name, Type: "Secret", Status: secStatus})
-
-		// Deployment status
-		depStatus := "Pending"
-		depImage := ""
-		if desiredDep.Name != "" {
-			if desiredDep.Status.ReadyReplicas > 0 {
-				depStatus = "Ready"
-			}
-			for _, c := range desiredDep.Spec.Template.Spec.Containers {
-				if c.Name == "agent" {
-					depImage = c.Image
-					break
-				}
-			}
-		}
-		resourcesStatus = append(resourcesStatus, v1alpha1.Resource{Name: desiredDep.Name, Type: "Deployment", Status: depStatus, Image: depImage})
-
-		// Update status if changed
-		needUpdate := false
-		if wireguard.Status.UniqueIdentifier != uniqueIdentifier {
-			needUpdate = true
-		}
-		if len(wireguard.Status.Resources) != len(resourcesStatus) {
-			needUpdate = true
-		} else {
-			for i := range resourcesStatus {
-				if wireguard.Status.Resources[i] != resourcesStatus[i] {
-					needUpdate = true
-					break
-				}
-			}
-		}
-		if needUpdate {
-			latest := &v1alpha1.Wireguard{}
-			if err := r.Get(ctx, req.NamespacedName, latest); err == nil {
-				latest.Status.UniqueIdentifier = uniqueIdentifier
-				latest.Status.Resources = resourcesStatus
-				if err := r.Status().Update(ctx, latest); err != nil {
-					if errors.IsNotFound(err) || errors.IsConflict(err) {
-						// ignore; continue
-					} else {
-						log.Error(err, "Failed to update wireguard resource status")
-						return ctrl.Result{}, err
-					}
-				}
-			}
-		}
-	}
-
 	if err := r.updateWireguardPeers(ctx, req, wireguard); err != nil {
 		return ctrl.Result{}, err
 	}
diff --git a/internal/resources/service.go b/internal/resources/service.go
index ede8010..dc5bffc 100644
--- a/internal/resources/service.go
+++ b/internal/resources/service.go
@@ -39,38 +39,28 @@ func NewServiceBuilder(scheme *runtime.Scheme) *ServiceBuilder {
 	return &ServiceBuilder{scheme: scheme}
 }
 
-// ForWireguard creates a service for a Wireguard server.
-func (b *ServiceBuilder) ForWireguard(wg *v1alpha1.Wireguard, serviceType corev1.ServiceType) (*corev1.Service, error) {
+// ForWireguard creates the ClusterIP service for a Wireguard server.
+func (b *ServiceBuilder) ForWireguard(wg *v1alpha1.Wireguard) (*corev1.Service, error) {
 	labels := LabelsForWireguard(wg.Name)
 
-	svcPorts := []corev1.ServicePort{{
-		Name:       "wireguard",
-		Protocol:   corev1.ProtocolUDP,
-		NodePort:   wg.Spec.NodePort,
-		Port:       WireguardPort,
-		TargetPort: intstr.FromInt(WireguardPort),
-	}}
-
 	svc := &corev1.Service{
 		ObjectMeta: metav1.ObjectMeta{
-			Name:        wg.Name + "-svc",
-			Namespace:   wg.Namespace,
-			Annotations: wg.Spec.ServiceAnnotations,
-			Labels:      labels,
+			Name:      wg.Name + "-svc",
+			Namespace: wg.Namespace,
+			Labels:    labels,
 		},
 		Spec: corev1.ServiceSpec{
 			Selector: labels,
-			Ports:    svcPorts,
-			Type:     serviceType,
+			Ports: []corev1.ServicePort{{
+				Name:       "wireguard",
+				Protocol:   corev1.ProtocolUDP,
+				Port:       WireguardPort,
+				TargetPort: intstr.FromInt(WireguardPort),
+			}},
+			Type: corev1.ServiceTypeClusterIP,
 		},
 	}
 
-	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
-		if wg.Spec.Address != "" {
-			svc.Spec.LoadBalancerIP = wg.Spec.Address
-		}
-	}
-
 	if err := SetOwnerReference(wg, svc, b.scheme); err != nil {
 		return nil, err
 	}
~~~~

```bash
git rm -q examples/serverWithNodePortService.yaml
```

- [ ] **Step 7: Regenerate**

```bash
make manifests generate &&
AGENT=$(sed -n 's/^[[:space:]]*- --agent-image=//p' release.yaml | head -1) &&
MGR=$(sed -n 's/^        image: //p' release.yaml | head -1) &&
make generate-release-file AGENT_IMAGE="$AGENT" MANAGER_IMAGE="$MGR" > /dev/null &&
echo regen-ok
```

Expected: the block's last line is `regen-ok`. Anything else (a controller-gen or kustomize error) is `PLAN: BLOCKED — every path is a guess: regenerate`.

- [ ] **Step 8: The builder test passes**

Run: `go test ./internal/resources/ -count=1 -v`
Expected: exactly two `--- PASS` lines, `TestAgentContainerExposesOnlyWireguardAndHealthPorts` and `TestServiceIsClusterIPWithOnlyTheWireguardPort`, then `ok`.

- [ ] **Step 9: Build and vet every build tag**

Run: `go build ./... && go vet ./... && go vet -tags=integration ./... && go vet -tags=e2e ./...`
Expected: no output, exit 0. A compile error here means a patch step was skipped or reordered: use the restart in Global Constraints.

- [ ] **Step 10: Unit tests**

Run: `go test ./... -race -count=1`
Expected: no `FAIL` and no `[FAIL]` anywhere in the output. Raw output shows `ok` per tested package; under the RTK hook it shows one `Go test: <n> passed in <m> packages` line.

- [ ] **Step 11: Integration suite (envtest)**

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race`
Expected: `ok  	github.com/jacaudi/wireguard-operator/internal/controller` after roughly 90 s; no `FAIL`. Known timing flake: if the only failure is the spec `no update churn applies each owned object at most once per reconcile and persists nothing` with the message `a reconcile was still in flight`, rerun the suite once. It failed 1 run in about 15 during this plan's verification, passed 3/3 on rerun of the same commit, and did not reproduce in the plan gate's 23 runs. A second failure, or any other failing spec, is real.

- [ ] **Step 12: Lint**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...`
Expected: the last line is exactly `0 issues.` Gate on that line, never on the exit status (the RTK hook swallows it).

- [ ] **Step 13: Format check (CI's `gofmt / goimports check`)**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint fmt --diff; echo fmt-exit=$?`
Expected: only `fmt-exit=0`. Any diff printed above it fails CI's required `ci` check even when lint says `0 issues.` (the lint ratchet does not see formatting); run `gofmt -w` on the files it names, rerun, and keep the result in this task's commit.

- [ ] **Step 14: YAML lint**

Run: `yamllint .`
Expected: no line containing `error`. One pre-existing `warning` for `./taskfile.yml` 52:3 is normal.

- [ ] **Step 15: Commit**

```bash
git add -A -- api config examples internal release.yaml
git commit -m "refactor!: make the server Service always ClusterIP" -m "LoadBalancer, NodePort, address, externalAddress and service annotations existed for external clients (design 2.2). Their status fields go too; status.endpoint replaces them in M2." -m "BREAKING CHANGE: spec.serviceType, spec.port, spec.address, spec.externalAddress, spec.serviceAnnotations, status.address, status.port, status.uniqueIdentifier and status.resources are removed; \`<name>-svc\` is always ClusterIP."
```

- [ ] **Step 16: Confirm the tree is clean and the commit is signed**

Run: `git status --porcelain`
Expected: no output. If anything is listed, it is a path this task touched but the `git add` above missed: `git add` it, `git commit --amend --no-edit`, rerun this step. A second non-empty result is `PLAN: BLOCKED — every path is a guess: dirty tree after commit`.

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. Any other value, a signing error, or a hang at the commit step is `PLAN: BLOCKED` (never `--no-gpg-sign`).

- [ ] **Step 17: Generated files and release.yaml still match their sources**

Run: `make -s manifests generate && git status --porcelain`
Expected: no output. If files are listed, the regenerate block (Task 2 Step 5) was skipped or an API edit came after it: `git add -A -- api config && git commit --amend --no-edit`, rerun. A second failure is `PLAN: BLOCKED — every path is a guess: codegen drift`.

Run: `sh hack/release-file-drift.sh`
Expected: the last line is `release.yaml matches config/`. If not, rerun the regenerate block (Task 2 Step 5 shows it), `git add release.yaml && git commit --amend --no-edit`, rerun this check. A second failure is `PLAN: BLOCKED — every path is a guess: release.yaml drift`.

---

### Task 7: Delete the one-value knobs and the vestigial ConfigMap

Kind: refactor

**Files:**
- Create: `internal/wireguard/wireguard_test.go`
- Delete: `internal/resources/configmap.go`, `examples/serverWithIpForwardEnable.yaml`
- Modify: `api/v1alpha1/wireguard_types.go` (`EnableIpForwardOnPodInit`, `UseWgUserspaceImplementation`), `internal/resources/deployment.go` (sysctl init always present; no forced-userspace flag; no `envFrom`), `internal/resources/deployment_test.go`, `internal/controller/wireguard_controller.go` (ConfigMap create, `configMapBuilder`, `Owns(&corev1.ConfigMap{})`, the `configmaps` RBAC marker), `cmd/agent/main.go` (`--wg-use-userspace-implementation`), `internal/wireguard/wireguard.go` (`SyncLink`'s forced-userspace branch, `Wireguard.WgUseUserspaceImpl`), `internal/controller/wireguard_controller_test.go`
- Regenerated: `vpn.wireguard-operator.io_wireguards.yaml`, `config/rbac/role.yaml` (drops `configmaps`), `release.yaml`

**Interfaces:**
- Consumes: Task 2's `buildDeployment`/`testWireguard` helpers.
- Produces:
  - `func SyncLink(_ agent.State, iface string, wgUserspaceImplementationFallback string) error`: kernel module first, `wireguard-go` fallback, then `waitForLink(iface, userspaceLinkTimeout)` (5 s, 100 ms polls) so the first sync on a node without the kernel module succeeds (Ruling 3). `func waitForLink(iface string, timeout time.Duration) error` returns an error after `timeout` whose text keeps the old hint, `ensure a userspace implementation (e.g., wireguard-go) is running for this interface`.
  - The `api/v1alpha1/wireguard_types.go` struct is left gofmt-aligned by the patch.
  - `wireguard.Wireguard{Logger, Iface, ListenPort, WgUserspaceImplementationFallback}`.
  - The Deployment always carries the privileged `sysctl` init container (`echo 1 > /proc/sys/net/ipv4/ip_forward`); this is divergence 2.
  - `resources.ConfigMapBuilder` no longer exists.

- [ ] **Step 1: Remove the userspace-knob specs**

```bash
perl -i -ne 'print unless /^\t\tfor _, useWgUserspace := range \[\]bool\{true, false\} \{$/ .. /^\t\t\}$/' internal/controller/wireguard_controller_test.go
gofmt -w internal/controller/wireguard_controller_test.go
```

Run: `cat internal/controller/wireguard_controller_test.go | grep -cF -e 'useWgUserspace'`
Expected: `0`. A non-zero count means an anchor did not match (the file is not in the state this plan assumes): use the restart in Global Constraints; if it recurs, `PLAN: BLOCKED — every path is a guess: plan drift <task>/<step>`.

- [ ] **Step 2: Write the failing Deployment test and drop the now-unused `fmt` import**

```bash
awk -v id=t7-test '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t7-test -->
~~~~diff
diff --git a/internal/controller/wireguard_controller_test.go b/internal/controller/wireguard_controller_test.go
index d721e4b..695e234 100644
--- a/internal/controller/wireguard_controller_test.go
+++ b/internal/controller/wireguard_controller_test.go
@@ -4,7 +4,6 @@ package controllers
 
 import (
 	"context"
-	"fmt"
 	"slices"
 	"time"
 
diff --git a/internal/resources/deployment_test.go b/internal/resources/deployment_test.go
index a4b51ae..58470e1 100644
--- a/internal/resources/deployment_test.go
+++ b/internal/resources/deployment_test.go
@@ -2,6 +2,7 @@ package resources
 
 import (
 	"maps"
+	"slices"
 	"testing"
 
 	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
@@ -48,3 +49,23 @@ func TestAgentContainerExposesOnlyWireguardAndHealthPorts(t *testing.T) {
 		t.Errorf("agent ports = %v, want %v", got, want)
 	}
 }
+
+func TestDeploymentAlwaysEnablesIPForwardingAndReadsNoConfigMap(t *testing.T) {
+	spec := buildDeployment(t, testWireguard()).Spec.Template.Spec
+
+	if len(spec.InitContainers) != 1 || spec.InitContainers[0].Name != "sysctl" {
+		t.Fatalf("init containers = %v, want exactly [sysctl]", spec.InitContainers)
+	}
+	sc := spec.InitContainers[0].SecurityContext
+	if sc == nil || sc.Privileged == nil || !*sc.Privileged {
+		t.Errorf("sysctl init container must be privileged, got %+v", sc)
+	}
+
+	agent := spec.Containers[0]
+	if len(agent.EnvFrom) != 0 {
+		t.Errorf("agent envFrom = %v, want none", agent.EnvFrom)
+	}
+	if slices.Contains(agent.Command, "--wg-use-userspace-implementation") {
+		t.Errorf("agent command %v must not force the userspace implementation", agent.Command)
+	}
+}
~~~~

- [ ] **Step 3: Watch it fail**

Run: `go test ./internal/resources/ -run TestDeploymentAlwaysEnablesIPForwardingAndReadsNoConfigMap -count=1`
Expected: FAIL with `init containers = [], want exactly [sysctl]`.

- [ ] **Step 4: Write the failing `waitForLink` test**

```bash
awk -v id=t7-wireguard-test '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md > internal/wireguard/wireguard_test.go
```

<!-- patch t7-wireguard-test -->
~~~~go
package wireguard

import (
	"strings"
	"testing"
	"time"
)

func TestWaitForLinkGivesUpAfterTheTimeout(t *testing.T) {
	const timeout = 300 * time.Millisecond

	start := time.Now()
	err := waitForLink("gate-none0", timeout)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("waitForLink returned nil for a link that never appears")
	}
	if elapsed < timeout {
		t.Errorf("waitForLink gave up after %s, want at least %s", elapsed, timeout)
	}
	if !strings.Contains(err.Error(), "ensure a userspace implementation") {
		t.Errorf("error %q lacks the actionable hint", err)
	}
}
~~~~

Run: `wc -l < internal/wireguard/wireguard_test.go | tr -d ' '`
Expected: `25`. Anything else means the extraction picked up the wrong block: delete the file and rerun the step.

- [ ] **Step 5: Watch it fail**

Run: `go test ./internal/wireguard/ -count=1`
Expected: build failure `undefined: waitForLink`. The test uses a link name that never exists, so it behaves the same on darwin (netlink returns `ErrNotImplemented`) and on Linux (link not found).

- [ ] **Step 6: Delete the knobs and the ConfigMap**

```bash
awk -v id=t7-impl '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t7-impl -->
~~~~diff
diff --git a/api/v1alpha1/wireguard_types.go b/api/v1alpha1/wireguard_types.go
index 82d5cd8..dda14ab 100644
--- a/api/v1alpha1/wireguard_types.go
+++ b/api/v1alpha1/wireguard_types.go
@@ -32,12 +32,8 @@ const (
 type WireguardSpec struct {
 	// PeerCIDR is the IPv4 CIDR range from which Wireguard peer addresses will be allocated.
 	// If omitted, the operator will default to 10.8.0.0/24.
-	PeerCIDR string `json:"peerCIDR,omitempty"`
-	// A boolean field that specifies whether IP forwarding should be enabled on the Wireguard VPN pod at startup. This can be useful to enable if the peers are having problems with sending traffic to the internet.
-	EnableIpForwardOnPodInit bool `json:"enableIpForwardOnPodInit,omitempty"`
-	// A boolean field that specifies whether to use the userspace implementation of Wireguard instead of the kernel one.
-	UseWgUserspaceImplementation bool              `json:"useWgUserspaceImplementation,omitempty"`
-	NodeSelector                 map[string]string `json:"nodeSelector,omitempty"`
+	PeerCIDR     string            `json:"peerCIDR,omitempty"`
+	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
 	// A list of Kubernetes taint tolerations applied to the Wireguard pod.
 	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
 	Agent       WireguardPodSpec    `json:"agent,omitempty"`
diff --git a/cmd/agent/main.go b/cmd/agent/main.go
index 64abca0..4da611b 100644
--- a/cmd/agent/main.go
+++ b/cmd/agent/main.go
@@ -22,13 +22,11 @@ func main() {
 	var verbosity int
 	var wgUserspaceImplementationFallback string
 	var wireguardListenPort int
-	var wgUseUserspaceImpl bool
 	flag.StringVar(&configFilePath, "state", "./state.json", "The location of the file that states the desired state")
 	flag.StringVar(&iface, "wg-iface", "wg0", "the wg device name. Default is wg0")
 	flag.StringVar(&wgUserspaceImplementationFallback, "wg-userspace-implementation-fallback", "wireguard-go", "The userspace implementation of wireguard to fallback to")
 	flag.IntVar(&wireguardListenPort, "wg-listen-port", 51820, "the UDP port wireguard is listening on")
 	flag.IntVar(&verbosity, "v", 1, "the verbosity level")
-	flag.BoolVar(&wgUseUserspaceImpl, "wg-use-userspace-implementation", false, "Use userspace implementation")
 	flag.Parse()
 
 	println(fmt.Sprintf(
@@ -40,7 +38,7 @@ func main() {
           .::::::::::::::::::::::!J#@@@@@@@BBBGPPG7:::::::::::::::::.     wg-iface: %s      
           .:::::::::::::::::::::^?Y5#@@@@@@5^...:::::::::::::::::::::     state: %s      
           .::::::::::::::::::::::..:!7Y#@@@@@#Y~:.::::::::::::::::::.     wg-listen-port: %d      
-          .:::::::::::::::::::.:^!?JYYJ?JG&@@@@@#7::::::::::::::::::.     wg-use-userspace-implementation: %v      
+          .:::::::::::::::::::.:^!?JYYJ?JG&@@@@@#7::::::::::::::::::.
           .:::::::::::::::::.^J#@@@@@@@@@&#B&@@@@@G:::::::::::::::::.     wg-userspace-implementation-fallback: %s           
           .:::::::::::::::::J@@@@@@@@@@@@@@@&G@@@@@J.:::::::::::::::.           
           .::::::::::::::::5@@@@@#?~~~7P@@@@@&B@@@@P.:::::::::::::::.           
@@ -65,7 +63,7 @@ func main() {
 	 \        /\    \_\  \    /    |    \\___  /\  ___/|   |  |  |   
 	  \__/\  /  \______  /    \____|__  /_____/  \___  |___|  |__|   
 		   \/          \/             \/             \/     \/
-`, iface, configFilePath, wireguardListenPort, wgUseUserspaceImpl, wgUserspaceImplementationFallback))
+`, iface, configFilePath, wireguardListenPort, wgUserspaceImplementationFallback))
 
 	stdr.SetVerbosity(verbosity)
 	log := stdr.NewWithOptions(log.New(os.Stderr, "", log.LstdFlags), stdr.Options{LogCaller: stdr.All})
@@ -76,7 +74,6 @@ func main() {
 		Iface:                             iface,
 		ListenPort:                        wireguardListenPort,
 		WgUserspaceImplementationFallback: wgUserspaceImplementationFallback,
-		WgUseUserspaceImpl:                wgUseUserspaceImpl,
 	}
 
 	close, err := agent.OnStateChange(configFilePath, log.WithName("onStateChange"), func(state agent.State) {
diff --git a/internal/controller/wireguard_controller.go b/internal/controller/wireguard_controller.go
index 62bc8fc..fb32798 100644
--- a/internal/controller/wireguard_controller.go
+++ b/internal/controller/wireguard_controller.go
@@ -58,7 +58,6 @@ type WireguardReconciler struct {
 	secretBuilder     *resources.SecretBuilder
 	serviceBuilder    *resources.ServiceBuilder
 	deploymentBuilder *resources.DeploymentBuilder
-	configMapBuilder  *resources.ConfigMapBuilder
 	ipAllocator       *ipam.Allocator
 }
 
@@ -222,7 +221,6 @@ func (r *WireguardReconciler) markPeerPublicKeyMissing(ctx context.Context, peer
 
 //+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
 //+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
-//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
 //+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
 //+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
 //+kubebuilder:rbac:groups="apps",resources=pods,verbs=get;list;watch;create;update;patch;delete
@@ -409,31 +407,6 @@ func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (
 		return ctrl.Result{}, err
 	}
 
-	// configmap
-
-	configFound := &corev1.ConfigMap{}
-	err = r.Get(ctx, types.NamespacedName{Name: wireguard.Name + "-config", Namespace: wireguard.Namespace}, configFound)
-	if err != nil && errors.IsNotFound(err) {
-		config, err := r.configMapBuilder.ForWireguard(wireguard)
-		if err != nil {
-			log.Error(err, "Failed to build configmap", "wireguard.Name", wireguard.Name)
-			return ctrl.Result{}, err
-		}
-		log.Info("Creating a new config", "config.Namespace", config.Namespace, "config.Name", config.Name)
-		err = r.Create(ctx, config)
-		if err != nil {
-			log.Error(err, "Failed to create new dep", "dep.Namespace", config.Namespace, "dep.Name", config.Name)
-			return ctrl.Result{}, err
-		}
-
-		err = r.updateStatus(ctx, req, metav1.Condition{Type: ConditionProgressing, Status: metav1.ConditionTrue, Reason: "ConfigMapPending", Message: "Waiting for configmap to be created"})
-
-		return ctrl.Result{}, err
-	} else if err != nil {
-		log.Error(err, "Failed to get config")
-		return ctrl.Result{}, err
-	}
-
 	// deployment
 	//
 	// One build, one apply. The apiserver diffs the applied field set against the stored
@@ -467,14 +440,12 @@ func (r *WireguardReconciler) SetupWithManager(mgr ctrl.Manager) error {
 	r.secretBuilder = resources.NewSecretBuilder(r.Scheme)
 	r.serviceBuilder = resources.NewServiceBuilder(r.Scheme)
 	r.deploymentBuilder = resources.NewDeploymentBuilder(r.Scheme, r.AgentImage, r.AgentImagePullPolicy)
-	r.configMapBuilder = resources.NewConfigMapBuilder(r.Scheme)
 	r.ipAllocator = ipam.NewAllocator()
 
 	return ctrl.NewControllerManagedBy(mgr).
 		For(&v1alpha1.Wireguard{}).
 		Owns(&v1alpha1.WireguardPeer{}).
 		Owns(&corev1.Service{}).
-		Owns(&corev1.ConfigMap{}).
 		Owns(&appsv1.Deployment{}).
 		Owns(&corev1.Secret{}).
 		Complete(r)
diff --git a/internal/resources/deployment.go b/internal/resources/deployment.go
index 5bfd2f6..b5190db 100644
--- a/internal/resources/deployment.go
+++ b/internal/resources/deployment.go
@@ -98,7 +98,7 @@ func (b *DeploymentBuilder) ForWireguard(wg *v1alpha1.Wireguard) (*appsv1.Deploy
 							},
 						},
 					},
-					InitContainers: []corev1.Container{},
+					InitContainers: []corev1.Container{b.ipForwardInitContainer()},
 					Containers: []corev1.Container{
 						b.agentContainer(wg, readOnlyRootFilesystem, allowPrivilegeEscalation),
 					},
@@ -107,20 +107,6 @@ func (b *DeploymentBuilder) ForWireguard(wg *v1alpha1.Wireguard) (*appsv1.Deploy
 		},
 	}
 
-	// Add IP forwarding init container if requested
-	if wg.Spec.EnableIpForwardOnPodInit {
-		dep.Spec.Template.Spec.InitContainers = append(dep.Spec.Template.Spec.InitContainers, b.ipForwardInitContainer())
-	}
-
-	// Add userspace implementation flag if requested
-	if wg.Spec.UseWgUserspaceImplementation {
-		for i, c := range dep.Spec.Template.Spec.Containers {
-			if c.Name == "agent" {
-				dep.Spec.Template.Spec.Containers[i].Command = append(dep.Spec.Template.Spec.Containers[i].Command, "--wg-use-userspace-implementation")
-			}
-		}
-	}
-
 	if err := SetOwnerReference(wg, dep, b.scheme); err != nil {
 		return nil, err
 	}
@@ -159,13 +145,6 @@ func (b *DeploymentBuilder) agentContainer(wg *v1alpha1.Wireguard, readOnlyRootF
 				Protocol:      corev1.ProtocolTCP,
 			},
 		},
-		EnvFrom: []corev1.EnvFromSource{
-			{
-				ConfigMapRef: &corev1.ConfigMapEnvSource{
-					LocalObjectReference: corev1.LocalObjectReference{Name: wg.Name + "-config"},
-				},
-			},
-		},
 		ReadinessProbe: &corev1.Probe{
 			ProbeHandler: corev1.ProbeHandler{
 				HTTPGet: &corev1.HTTPGetAction{
diff --git a/internal/wireguard/wireguard.go b/internal/wireguard/wireguard.go
index d869de9..ebf68ce 100644
--- a/internal/wireguard/wireguard.go
+++ b/internal/wireguard/wireguard.go
@@ -133,6 +133,25 @@ func createLinkUsingUserspaceImpl(iface string, wgUserspaceImplementationFallbac
 	return nil
 }
 
+// userspaceLinkTimeout bounds how long SyncLink waits for the userspace implementation to
+// create the link it was started for.
+const userspaceLinkTimeout = 5 * time.Second
+
+// waitForLink polls until iface exists or timeout elapses.
+func waitForLink(iface string, timeout time.Duration) error {
+	deadline := time.Now().Add(timeout)
+	for {
+		_, err := netlink.LinkByName(iface)
+		if err == nil {
+			return nil
+		}
+		if time.Now().After(deadline) {
+			return fmt.Errorf("userspace WireGuard did not create link %q within %s: %w; ensure a userspace implementation (e.g., wireguard-go) is running for this interface", iface, timeout, err)
+		}
+		time.Sleep(100 * time.Millisecond)
+	}
+}
+
 func createLinkUsingKernalModule(iface string) error {
 	// link not created
 	wgLink := &netlink.GenericLink{
@@ -149,7 +168,9 @@ func createLinkUsingKernalModule(iface string) error {
 	return nil
 }
 
-func SyncLink(_ agent.State, iface string, wgUserspaceImplementationFallback string, wgUseUserspaceImpl bool) error {
+// SyncLink creates the WireGuard link iface if it does not exist, using the kernel module
+// and falling back to the userspace implementation when the module is unavailable.
+func SyncLink(_ agent.State, iface string, wgUserspaceImplementationFallback string) error {
 	_, err := netlink.LinkByName(iface)
 	if err != nil {
 		if _, ok := err.(netlink.LinkNotFoundError); !ok {
@@ -158,43 +179,18 @@ func SyncLink(_ agent.State, iface string, wgUserspaceImplementationFallback str
 	}
 
 	if _, ok := err.(netlink.LinkNotFoundError); ok {
-		if wgUseUserspaceImpl {
+		err = createLinkUsingKernalModule(iface)
+
+		if err != nil {
 			err = createLinkUsingUserspaceImpl(iface, wgUserspaceImplementationFallback)
 
 			if err != nil {
 				return err
 			}
 
-			// Wait briefly for userspace implementation to create the link
-			var lastErr error
-			deadline := time.Now().Add(5 * time.Second)
-			for time.Now().Before(deadline) {
-				var l netlink.Link
-				l, lastErr = netlink.LinkByName(iface)
-				if lastErr == nil {
-					// Ensure link is up
-					if err := netlink.LinkSetUp(l); err != nil {
-						return fmt.Errorf("bringing link %q up failed: %w", iface, err)
-					}
-					lastErr = nil
-					break
-				}
-				time.Sleep(100 * time.Millisecond)
-			}
-
-			if lastErr != nil {
-				return fmt.Errorf("userspace WireGuard did not create link %q in time: %w; ensure a userspace implementation (e.g., wireguard-go) is running for this interface", iface, lastErr)
-			}
-
-		} else {
-			err = createLinkUsingKernalModule(iface)
-
-			if err != nil {
-				err = createLinkUsingUserspaceImpl(iface, wgUserspaceImplementationFallback)
-
-				if err != nil {
-					return err
-				}
+			// The userspace implementation creates the link asynchronously.
+			if err := waitForLink(iface, userspaceLinkTimeout); err != nil {
+				return err
 			}
 		}
 
@@ -241,13 +237,12 @@ type Wireguard struct {
 	Iface                             string
 	ListenPort                        int
 	WgUserspaceImplementationFallback string
-	WgUseUserspaceImpl                bool
 }
 
 func (wg *Wireguard) Sync(state agent.State) error {
 	wg.Logger.V(2).Info("syncing Wireguard")
 	// create wg0 link
-	err := SyncLink(state, wg.Iface, wg.WgUserspaceImplementationFallback, wg.WgUseUserspaceImpl)
+	err := SyncLink(state, wg.Iface, wg.WgUserspaceImplementationFallback)
 	if err != nil {
 		return err
 	}
~~~~

```bash
git rm -q internal/resources/configmap.go examples/serverWithIpForwardEnable.yaml
```

- [ ] **Step 7: Regenerate**

```bash
make manifests generate &&
AGENT=$(sed -n 's/^[[:space:]]*- --agent-image=//p' release.yaml | head -1) &&
MGR=$(sed -n 's/^        image: //p' release.yaml | head -1) &&
make generate-release-file AGENT_IMAGE="$AGENT" MANAGER_IMAGE="$MGR" > /dev/null &&
echo regen-ok
```

Expected: the block's last line is `regen-ok`. Anything else (a controller-gen or kustomize error) is `PLAN: BLOCKED — every path is a guess: regenerate`.

- [ ] **Step 8: The test passes**

Run: `go test ./internal/resources/ -count=1 -v`
Expected: exactly three `--- PASS` lines, `TestAgentContainerExposesOnlyWireguardAndHealthPorts`, `TestDeploymentAlwaysEnablesIPForwardingAndReadsNoConfigMap` and `TestServiceIsClusterIPWithOnlyTheWireguardPort`, then `ok`.

Run: `go test ./internal/wireguard/ -count=1 -v`
Expected: `--- PASS: TestWaitForLinkGivesUpAfterTheTimeout` (about 0.30 s), then `ok`.

- [ ] **Step 9: Build and vet every build tag**

Run: `go build ./... && go vet ./... && go vet -tags=integration ./... && go vet -tags=e2e ./...`
Expected: no output, exit 0. A compile error here means a patch step was skipped or reordered: use the restart in Global Constraints.

- [ ] **Step 10: Unit tests**

Run: `go test ./... -race -count=1`
Expected: no `FAIL` and no `[FAIL]` anywhere in the output. Raw output shows `ok` per tested package; under the RTK hook it shows one `Go test: <n> passed in <m> packages` line.

- [ ] **Step 11: Integration suite (envtest)**

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race`
Expected: `ok  	github.com/jacaudi/wireguard-operator/internal/controller` after roughly 90 s; no `FAIL`. Known timing flake: if the only failure is the spec `no update churn applies each owned object at most once per reconcile and persists nothing` with the message `a reconcile was still in flight`, rerun the suite once. It failed 1 run in about 15 during this plan's verification, passed 3/3 on rerun of the same commit, and did not reproduce in the plan gate's 23 runs. A second failure, or any other failing spec, is real.

- [ ] **Step 12: Lint**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...`
Expected: the last line is exactly `0 issues.` Gate on that line, never on the exit status (the RTK hook swallows it).

- [ ] **Step 13: Format check (CI's `gofmt / goimports check`)**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint fmt --diff; echo fmt-exit=$?`
Expected: only `fmt-exit=0`. Any diff printed above it fails CI's required `ci` check even when lint says `0 issues.` (the lint ratchet does not see formatting); run `gofmt -w` on the files it names, rerun, and keep the result in this task's commit.

- [ ] **Step 14: YAML lint**

Run: `yamllint .`
Expected: no line containing `error`. One pre-existing `warning` for `./taskfile.yml` 52:3 is normal.

- [ ] **Step 15: Nothing reads an object the trimmed operator no longer manages (Review Focus 4)**

Run: `grep -rn 'metrics-svc\|"-config"\|peer-configs\|"-peer"\|ConfigMap' --include='*.go' api cmd internal | grep -v '_test.go:' ; echo done`
Expected: only `done`.

- [ ] **Step 16: Commit**

```bash
git add -A -- api cmd config examples internal release.yaml
git commit -m "refactor!: delete the one-value knobs and the vestigial ConfigMap" -m "useWgUserspaceImplementation, enableIpForwardOnPodInit and the agent's forced-userspace flag had one sensible value; the kernel-to-userspace fallback stays automatic (with a bounded wait for the link) and IP forwarding is now always enabled. The <name>-config ConfigMap carried no data (design 2.2)." -m "BREAKING CHANGE: spec.useWgUserspaceImplementation, spec.enableIpForwardOnPodInit and the agent flag --wg-use-userspace-implementation are removed; the server pod always runs a privileged sysctl init container."
```

- [ ] **Step 17: Confirm the tree is clean and the commit is signed**

Run: `git status --porcelain`
Expected: no output. If anything is listed, it is a path this task touched but the `git add` above missed: `git add` it, `git commit --amend --no-edit`, rerun this step. A second non-empty result is `PLAN: BLOCKED — every path is a guess: dirty tree after commit`.

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. Any other value, a signing error, or a hang at the commit step is `PLAN: BLOCKED` (never `--no-gpg-sign`).

- [ ] **Step 18: Generated files and release.yaml still match their sources**

Run: `make -s manifests generate && git status --porcelain`
Expected: no output. If files are listed, the regenerate block (Task 2 Step 5) was skipped or an API edit came after it: `git add -A -- api config && git commit --amend --no-edit`, rerun. A second failure is `PLAN: BLOCKED — every path is a guess: codegen drift`.

Run: `sh hack/release-file-drift.sh`
Expected: the last line is `release.yaml matches config/`. If not, rerun the regenerate block (Task 2 Step 5 shows it), `git add release.yaml && git commit --amend --no-edit`, rerun this check. A second failure is `PLAN: BLOCKED — every path is a guess: release.yaml drift`.

---

### Task 8: Delete the OLM bundle and the unused webhook server

Kind: refactor

**Files:**
- Delete: `bundle/` (7 files), `config/manifests/` (2 files), `config/scorecard/` (4 files)
- Modify: `Makefile` (the `VERSION`, `CHANNELS`, `DEFAULT_CHANNEL`, `BUNDLE_*` and `IMAGE_TAG_BASE` variables; divergence 5), `PROJECT` (the operator-sdk `manifests` and `scorecard` plugins), `cmd/manager/main.go` (`WebhookServer` on :9443; nothing registers a handler), `CONTRIBUTING.md` (no `bundle/`)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: the manager no longer opens port 9443. `make manifests`, `make generate`, `make generate-release-file` and `make run-e2e` keep working.

- [ ] **Step 1: Green baseline for the targets this touches**

Run: `make -n manifests generate > /dev/null && grep -cE '^(generate-release-file|run-e2e):' Makefile`
Expected: `2`. Never probe `generate-release-file` or `run-e2e` with `make -n`: their recipes call `$(MAKE)`, which GNU make executes even under `-n`, rewriting `release.yaml` and writing `release_it.yaml`.

- [ ] **Step 2: Delete the OLM tree**

```bash
git rm -rq bundle config/manifests config/scorecard
```

- [ ] **Step 3: Remove the bundle variables, the SDK plugins and the webhook server**

```bash
awk -v id=t8-impl '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t8-impl -->
~~~~diff
diff --git a/CONTRIBUTING.md b/CONTRIBUTING.md
index 9907d1c..0345efc 100644
--- a/CONTRIBUTING.md
+++ b/CONTRIBUTING.md
@@ -16,8 +16,8 @@ make generate    # regenerate deepcopy functions after changing api/
 
 `make test` regenerates before running, so a stale CRD or an unformatted file fails
 locally rather than in CI. Never hand-edit `zz_generated.deepcopy.go` or anything under
-`config/crd/bases` — regenerate instead. `release.yaml` and `bundle/` are produced at
-release time and should not be edited by hand either.
+`config/crd/bases` — regenerate instead. `release.yaml` is produced at release time and
+should not be edited by hand either.
 
 ## Engineering practices
 
diff --git a/Makefile b/Makefile
index d486824..e2ee923 100644
--- a/Makefile
+++ b/Makefile
@@ -1,9 +1,3 @@
-# VERSION defines the project version for the bundle.
-# Update this value when you upgrade the version of your project.
-# To re-generate a bundle for another specific version without changing the standard setup, you can:
-# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
-# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
-VERSION ?= 0.0.1
 SHELL = /bin/bash
 
 
@@ -40,36 +34,6 @@ KIND_VERSION ?= v0.23.0
 AGENT_IMAGE ?= "agent:dev"
 MANAGER_IMAGE ?= "manager:dev"
 
-# CHANNELS define the bundle channels used in the bundle.
-# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
-# To re-generate a bundle for other specific channels without changing the standard setup, you can:
-# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
-# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
-ifneq ($(origin CHANNELS), undefined)
-BUNDLE_CHANNELS := --channels=$(CHANNELS)
-endif
-
-# DEFAULT_CHANNEL defines the default channel used in the bundle.
-# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
-# To re-generate a bundle for any other default channel without changing the default setup, you can:
-# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
-# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
-ifneq ($(origin DEFAULT_CHANNEL), undefined)
-BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
-endif
-BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)
-
-# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
-# This variable is used to construct full image tags for bundle and catalog images.
-#
-# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
-# wireguard-operator.io/manager-bundle:$VERSION and wireguard-operator.io/manager-catalog:$VERSION.
-IMAGE_TAG_BASE ?= ghcr.io/jacaudi/wireguard-operator
-
-# BUNDLE_IMG defines the image:tag used for the bundle.
-# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
-BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-operator-bundle:main
-
 # ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
 ENVTEST_K8S_VERSION = 1.30.0
 
diff --git a/PROJECT b/PROJECT
index 55ae456..225c220 100644
--- a/PROJECT
+++ b/PROJECT
@@ -1,9 +1,6 @@
 domain: wireguard-operator.io
 layout:
 - go.kubebuilder.io/v3
-plugins:
-  manifests.sdk.operatorframework.io/v2: {}
-  scorecard.sdk.operatorframework.io/v2: {}
 projectName: wireguard-operator
 repo: github.com/jacaudi/wireguard-operator
 resources:
diff --git a/cmd/manager/main.go b/cmd/manager/main.go
index a46d726..0d6c75a 100644
--- a/cmd/manager/main.go
+++ b/cmd/manager/main.go
@@ -37,7 +37,6 @@ import (
 	"sigs.k8s.io/controller-runtime/pkg/log/zap"
 	metricsfilters "sigs.k8s.io/controller-runtime/pkg/metrics/filters"
 	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
-	"sigs.k8s.io/controller-runtime/pkg/webhook"
 	//+kubebuilder:scaffold:imports
 )
 
@@ -96,7 +95,6 @@ func main() {
 			BindAddress:    metricsAddr,
 			FilterProvider: metricsfilters.WithAuthenticationAndAuthorization,
 		},
-		WebhookServer:           webhook.NewServer(webhook.Options{Port: 9443}),
 		HealthProbeBindAddress:  probeAddr,
 		LeaderElection:          enableLeaderElection,
 		LeaderElectionID:        "a6d3bffc.wireguard-operator.io",
~~~~

- [ ] **Step 4: Regenerate (expect no change: nothing generated depends on these files)**

```bash
make manifests generate &&
AGENT=$(sed -n 's/^[[:space:]]*- --agent-image=//p' release.yaml | head -1) &&
MGR=$(sed -n 's/^        image: //p' release.yaml | head -1) &&
make generate-release-file AGENT_IMAGE="$AGENT" MANAGER_IMAGE="$MGR" > /dev/null &&
echo regen-ok
```

Expected: the block's last line is `regen-ok`. Anything else (a controller-gen or kustomize error) is `PLAN: BLOCKED — every path is a guess: regenerate`.

Run: `make -n manifests generate > /dev/null && grep -cE '^(generate-release-file|run-e2e):' Makefile; grep -rn 'bundle\|BUNDLE\|scorecard\|webhook\.' Makefile PROJECT cmd ; echo grep-exit=$?`
Expected: `2`, then only `grep-exit=1`.

Run: `git status --porcelain -- release.yaml`
Expected: no output (this task changes nothing generated).

- [ ] **Step 5: Build and vet every build tag**

Run: `go build ./... && go vet ./... && go vet -tags=integration ./... && go vet -tags=e2e ./...`
Expected: no output, exit 0. A compile error here means a patch step was skipped or reordered: use the restart in Global Constraints.

- [ ] **Step 6: Unit tests**

Run: `go test ./... -race -count=1`
Expected: no `FAIL` and no `[FAIL]` anywhere in the output. Raw output shows `ok` per tested package; under the RTK hook it shows one `Go test: <n> passed in <m> packages` line.

- [ ] **Step 7: Integration suite (envtest)**

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race`
Expected: `ok  	github.com/jacaudi/wireguard-operator/internal/controller` after roughly 90 s; no `FAIL`. Known timing flake: if the only failure is the spec `no update churn applies each owned object at most once per reconcile and persists nothing` with the message `a reconcile was still in flight`, rerun the suite once. It failed 1 run in about 15 during this plan's verification, passed 3/3 on rerun of the same commit, and did not reproduce in the plan gate's 23 runs. A second failure, or any other failing spec, is real.

- [ ] **Step 8: Lint**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...`
Expected: the last line is exactly `0 issues.` Gate on that line, never on the exit status (the RTK hook swallows it).

- [ ] **Step 9: Format check (CI's `gofmt / goimports check`)**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint fmt --diff; echo fmt-exit=$?`
Expected: only `fmt-exit=0`. Any diff printed above it fails CI's required `ci` check even when lint says `0 issues.` (the lint ratchet does not see formatting); run `gofmt -w` on the files it names, rerun, and keep the result in this task's commit.

- [ ] **Step 10: YAML lint**

Run: `yamllint .`
Expected: no line containing `error`. One pre-existing `warning` for `./taskfile.yml` 52:3 is normal.

- [ ] **Step 11: Commit**

```bash
git add -A -- Makefile PROJECT cmd CONTRIBUTING.md config
git commit -m "refactor: delete the OLM bundle and the unused webhook server" -m "The bundle was never published and no webhook handler is registered (design 2.2)."
```

- [ ] **Step 12: Confirm the tree is clean and the commit is signed**

Run: `git status --porcelain`
Expected: no output. If anything is listed, it is a path this task touched but the `git add` above missed: `git add` it, `git commit --amend --no-edit`, rerun this step. A second non-empty result is `PLAN: BLOCKED — every path is a guess: dirty tree after commit`.

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. Any other value, a signing error, or a hang at the commit step is `PLAN: BLOCKED` (never `--no-gpg-sign`).

- [ ] **Step 13: Generated files and release.yaml still match their sources**

Run: `make -s manifests generate && git status --porcelain`
Expected: no output. If files are listed, the regenerate block (Task 2 Step 5) was skipped or an API edit came after it: `git add -A -- api config && git commit --amend --no-edit`, rerun. A second failure is `PLAN: BLOCKED — every path is a guess: codegen drift`.

Run: `sh hack/release-file-drift.sh`
Expected: the last line is `release.yaml matches config/`. If not, rerun the regenerate block (Task 2 Step 5 shows it), `git add release.yaml && git commit --amend --no-edit`, rerun this check. A second failure is `PLAN: BLOCKED — every path is a guess: release.yaml drift`.

---

### Task 9: Make the README describe only what the trimmed operator does

Kind: refactor

**Files:**
- Modify: `README.md`. Remove the "Tested with" section, whose every caveat names a deleted field, and the deleted features from the feature list. Change the example server to `spec: {}`, give the example peer a `publicKey`, replace the "Peer configuration" section with a note that no client config or client private key is stored, and drop the `spec.persistentKeepalive` reference in "How to deploy".

**Interfaces:**
- Consumes: Tasks 2–7 (the README must only name fields that exist).
- Produces: nothing code depends on. M6 rewrites the README fully.

- [ ] **Step 1: Baseline: the README names deleted fields**

Run: `grep -c 'mtu\|serviceType\|enableIpForwardOnPodInit\|peer-configs\|persistentKeepalive\|PersistentKeepalive\|wstunnel\|peerCIDRv6\|egressNetworkPolicies' README.md`
Expected: a number greater than `0`.

- [ ] **Step 2: Apply the README edits**

```bash
awk -v id=t9-impl '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t9-impl -->
~~~~diff
diff --git a/README.md b/README.md
index 8911bda..98d5e8e 100644
--- a/README.md
+++ b/README.md
@@ -6,33 +6,16 @@ Painless deployment of wireguard on kubernetes
 
 If you are facing any problems please open an [issue](https://github.com/jacaudi/wireguard-operator/issues)
 
-## Tested with
-- [x] IBM Cloud Kubernetes Service
-- [x] Gcore Labs KMP
-  * requires `spec.enableIpForwardOnPodInit: true`
-- [x] Google Kubernetes Engine
-  * requires `spec.mtu: "1380"`
-  * Not compatible with "Container-Optimized OS with containerd" node images
-  * Not compatible with autopilot
-- [x] DigitalOcean Kubernetes
-  * requires `spec.serviceType: "NodePort"`. DigitalOcean LoadBalancer does not support UDP. 
-- [ ] Amazon EKS
-- [ ] Azure Kubernetes Service
-- [ ] ...?
-
 ## Architecture 
 
 ![alt text](./readme/main.png)
 
 ## Features 
 * Falls back to userspace implementation of wireguard [wireguard-go](https://github.com/WireGuard/wireguard-go) if wireguard kernal module is missing
-* Automatic key generation
-* Automatic IP allocation
-* Does not need persistance. peer/server keys are stored as k8s secrets and loaded into the wireguard pod
-* Exposes a metrics endpoint
-* Supports tunneling/traffic obfuscation using [wstunnel](https://github.com/erebe/wstunnel)
-* IPv6 support, including IPv6-only peers, through `spec.peerCIDRv6` and `spec.ipv6Only`
-* Per-peer egress network policies through `WireguardPeer.spec.egressNetworkPolicies`
+* Automatic server key generation
+* Automatic IP allocation for peers
+* Does not need persistance. The server key is stored as a k8s secret and loaded into the wireguard pod
+* The server is reachable in-cluster only, through a ClusterIP Service on UDP 51820
 
 ## Example
 
@@ -43,8 +26,7 @@ apiVersion: vpn.wireguard-operator.io/v1alpha1
 kind: Wireguard
 metadata:
   name: "my-cool-vpn"
-spec:
-  mtu: "1380"
+spec: {}
 ```
 
 ### Peer
@@ -56,34 +38,14 @@ metadata:
   name: peer1
 spec:
   wireguardRef: "my-cool-vpn"
+  # The public half of a key the client generated itself: wg genkey | wg pubkey
+  publicKey: "v5Q+oRNBfxafhiAR9bXGEhRsRFfEFb7XsFV8DWGrykQ="
 ```
 
-#### Peer configuration
-
-Peer configurations are stored in a Secret named `<wireguard-name>-peer-configs`, one key per peer. Retrieve and decode like this:
-
-```console
-kubectl get secret my-cool-vpn-peer-configs -o jsonpath='{.data.peer1}' | base64 -d
-```
-
-Use the output to configure your preferred Wireguard client:
-
-```console
-[Interface]
-PrivateKey = WOhR7uTMAqmZamc1umzfwm8o4ZxLdR5LjDcUYaW/PH8=
-Address = 10.8.0.3
-DNS = 10.48.0.10, default.svc.cluster.local
-MTU = 1380
-
-[Peer]
-PublicKey = sO3ZWhnIT8owcdsfwiMRu2D8LzKmae2gUAxAmhx5GTg=
-AllowedIPs = 0.0.0.0/0
-Endpoint = 32.121.45.102:51820
-PersistentKeepalive = 25
-```
-
-`PersistentKeepalive` defaults to 25 seconds and is configurable through
-`Wireguard.Spec.PersistentKeepalive`. Set it to `0` to omit it.
+The operator stores no client configuration and no client private key: each peer
+generates its own key and registers only the public half. The in-pod client that
+consumes this is designed in
+[`docs/designs/2026-10-29-minimal-vpn-gateway-design.md`](docs/designs/2026-10-29-minimal-vpn-gateway-design.md).
 
 ## How to deploy
 
@@ -93,9 +55,8 @@ PersistentKeepalive = 25
 > from source.
 >
 > Earlier revisions of this README pointed at upstream's release and chart. Those
-> install the diverging upstream operator, which does not carry this fork's features —
-> `spec.persistentKeepalive`, for example — so they have been removed rather than
-> repointed.
+> install the diverging upstream operator, which is a different product from this fork,
+> so they have been removed rather than repointed.
 
 ```
 make deploy
~~~~

- [ ] **Step 3: No deleted field is named any more**

Run: `grep -c 'mtu\|serviceType\|enableIpForwardOnPodInit\|peer-configs\|persistentKeepalive\|PersistentKeepalive\|wstunnel\|peerCIDRv6\|egressNetworkPolicies' README.md`
Expected: `0`.

Run: `test -f docs/designs/2026-10-29-minimal-vpn-gateway-design.md && echo link-target-present`
Expected: `link-target-present` (the README now links to it).

- [ ] **Step 4: Build and vet every build tag**

Run: `go build ./... && go vet ./... && go vet -tags=integration ./... && go vet -tags=e2e ./...`
Expected: no output, exit 0. A compile error here means a patch step was skipped or reordered: use the restart in Global Constraints.

- [ ] **Step 5: Unit tests**

Run: `go test ./... -race -count=1`
Expected: no `FAIL` and no `[FAIL]` anywhere in the output. Raw output shows `ok` per tested package; under the RTK hook it shows one `Go test: <n> passed in <m> packages` line.

- [ ] **Step 6: Integration suite (envtest)**

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race`
Expected: `ok  	github.com/jacaudi/wireguard-operator/internal/controller` after roughly 90 s; no `FAIL`. Known timing flake: if the only failure is the spec `no update churn applies each owned object at most once per reconcile and persists nothing` with the message `a reconcile was still in flight`, rerun the suite once. It failed 1 run in about 15 during this plan's verification, passed 3/3 on rerun of the same commit, and did not reproduce in the plan gate's 23 runs. A second failure, or any other failing spec, is real.

- [ ] **Step 7: Lint**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...`
Expected: the last line is exactly `0 issues.` Gate on that line, never on the exit status (the RTK hook swallows it).

- [ ] **Step 8: Format check (CI's `gofmt / goimports check`)**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint fmt --diff; echo fmt-exit=$?`
Expected: only `fmt-exit=0`. Any diff printed above it fails CI's required `ci` check even when lint says `0 issues.` (the lint ratchet does not see formatting); run `gofmt -w` on the files it names, rerun, and keep the result in this task's commit.

- [ ] **Step 9: YAML lint**

Run: `yamllint .`
Expected: no line containing `error`. One pre-existing `warning` for `./taskfile.yml` 52:3 is normal.

- [ ] **Step 10: Commit**

```bash
git add -A -- README.md
git commit -m "docs: describe only what the trimmed operator does" -m "The README advertised wstunnel, IPv6, metrics, peer config Secrets and per-platform knobs that M1 deleted."
```

- [ ] **Step 11: Confirm the tree is clean and the commit is signed**

Run: `git status --porcelain`
Expected: no output. If anything is listed, it is a path this task touched but the `git add` above missed: `git add` it, `git commit --amend --no-edit`, rerun this step. A second non-empty result is `PLAN: BLOCKED — every path is a guess: dirty tree after commit`.

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. Any other value, a signing error, or a hang at the commit step is `PLAN: BLOCKED` (never `--no-gpg-sign`).

- [ ] **Step 12: Generated files and release.yaml still match their sources**

Run: `make -s manifests generate && git status --porcelain`
Expected: no output. If files are listed, the regenerate block (Task 2 Step 5) was skipped or an API edit came after it: `git add -A -- api config && git commit --amend --no-edit`, rerun. A second failure is `PLAN: BLOCKED — every path is a guess: codegen drift`.

Run: `sh hack/release-file-drift.sh`
Expected: the last line is `release.yaml matches config/`. If not, rerun the regenerate block (Task 2 Step 5 shows it), `git add release.yaml && git commit --amend --no-edit`, rerun this check. A second failure is `PLAN: BLOCKED — every path is a guess: release.yaml drift`.

---

### Task 10: Add iproute2 to the agent image

Kind: refactor

**Files:**
- Modify: `images/agent/Dockerfile` (`apt-get install … iproute2 iptables wireguard-tools`)

**Interfaces:**
- Consumes: Task 2's Dockerfile (the dead `PROMETHEUS_WIREGUARD_EXPORTER_SRC` ARGs are gone).
- Produces: the agent image ships `/usr/sbin/ip`. M4's exit node and M5's client both run `ip` from this image, and trixie's `wireguard-tools` does not pull it in (design §2.3, §4).

- [ ] **Step 1: Baseline**

Run: `grep -c 'iproute2' images/agent/Dockerfile`
Expected: `0`.

- [ ] **Step 2: Add the package**

```bash
awk -v id=t10-impl '$0=="<!-- patch " id " -->"{getline; f=1; next} f&&$0=="~~~~"{exit} f' docs/plans/2026-10-29-m1-trim-implementation.md | git apply
```

<!-- patch t10-impl -->
~~~~diff
diff --git a/images/agent/Dockerfile b/images/agent/Dockerfile
index f714796..6bbbf69 100644
--- a/images/agent/Dockerfile
+++ b/images/agent/Dockerfile
@@ -36,7 +36,7 @@ ARG WIREGUARD_GO_SRC
 ARG WIREGUARD_AGENT_SRC
 
 RUN apt-get update \
-    && apt-get install --no-install-recommends -y iptables wireguard-tools \
+    && apt-get install --no-install-recommends -y iproute2 iptables wireguard-tools \
     && rm -rf /var/lib/apt/lists/*
 
 COPY --from=golang-builder $WIREGUARD_GO_SRC/wireguard-go /usr/local/bin
~~~~

- [ ] **Step 3: Dockerfile lint**

Run: `hadolint images/agent/Dockerfile images/manager/Dockerfile; echo hadolint-exit=$?`
Expected: only `hadolint-exit=0`.

- [ ] **Step 4: The image builds and carries `ip` (needs Docker and network)**

Run: `docker build -q -t m1-agent:verify -f images/agent/Dockerfile . && docker run --rm --entrypoint sh m1-agent:verify -c 'for b in ip iptables wg wireguard-go agent; do command -v $b; done' && docker image rm -f m1-agent:verify`
Expected: a `sha256:` image ID line (from `docker build -q`), then five paths, `/usr/sbin/ip`, `/usr/sbin/iptables`, `/usr/bin/wg`, `/usr/local/bin/wireguard-go`, `/usr/local/bin/agent`, then `Untagged: m1-agent:verify` and a `Deleted:` line. If `docker info` fails (no daemon), record `unverified: docker unavailable` in the task report and continue; hadolint (Step 3) still gates. A build failure with a daemon present is a real failure. Retry once, because a parallel heavy job once caused a spurious `go build` exit 1 during verification, then stop.

- [ ] **Step 5: Build and vet every build tag**

Run: `go build ./... && go vet ./... && go vet -tags=integration ./... && go vet -tags=e2e ./...`
Expected: no output, exit 0. A compile error here means a patch step was skipped or reordered: use the restart in Global Constraints.

- [ ] **Step 6: Unit tests**

Run: `go test ./... -race -count=1`
Expected: no `FAIL` and no `[FAIL]` anywhere in the output. Raw output shows `ok` per tested package; under the RTK hook it shows one `Go test: <n> passed in <m> packages` line.

- [ ] **Step 7: Integration suite (envtest)**

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race`
Expected: `ok  	github.com/jacaudi/wireguard-operator/internal/controller` after roughly 90 s; no `FAIL`. Known timing flake: if the only failure is the spec `no update churn applies each owned object at most once per reconcile and persists nothing` with the message `a reconcile was still in flight`, rerun the suite once. It failed 1 run in about 15 during this plan's verification, passed 3/3 on rerun of the same commit, and did not reproduce in the plan gate's 23 runs. A second failure, or any other failing spec, is real.

- [ ] **Step 8: Lint**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...`
Expected: the last line is exactly `0 issues.` Gate on that line, never on the exit status (the RTK hook swallows it).

- [ ] **Step 9: Format check (CI's `gofmt / goimports check`)**

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint fmt --diff; echo fmt-exit=$?`
Expected: only `fmt-exit=0`. Any diff printed above it fails CI's required `ci` check even when lint says `0 issues.` (the lint ratchet does not see formatting); run `gofmt -w` on the files it names, rerun, and keep the result in this task's commit.

- [ ] **Step 10: YAML lint**

Run: `yamllint .`
Expected: no line containing `error`. One pre-existing `warning` for `./taskfile.yml` 52:3 is normal.

- [ ] **Step 11: Commit**

```bash
git add -A -- images/agent/Dockerfile
git commit -m "build(agent): add iproute2 to the agent image" -m "M4's exit node and M5's client run ip from this image; trixie's wireguard-tools does not pull iproute2 in (design 2.3)."
```

- [ ] **Step 12: Confirm the tree is clean and the commit is signed**

Run: `git status --porcelain`
Expected: no output. If anything is listed, it is a path this task touched but the `git add` above missed: `git add` it, `git commit --amend --no-edit`, rerun this step. A second non-empty result is `PLAN: BLOCKED — every path is a guess: dirty tree after commit`.

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. Any other value, a signing error, or a hang at the commit step is `PLAN: BLOCKED` (never `--no-gpg-sign`).

- [ ] **Step 13: Generated files and release.yaml still match their sources**

Run: `make -s manifests generate && git status --porcelain`
Expected: no output. If files are listed, the regenerate block (Task 2 Step 5) was skipped or an API edit came after it: `git add -A -- api config && git commit --amend --no-edit`, rerun. A second failure is `PLAN: BLOCKED — every path is a guess: codegen drift`.

Run: `sh hack/release-file-drift.sh`
Expected: the last line is `release.yaml matches config/`. If not, rerun the regenerate block (Task 2 Step 5 shows it), `git add release.yaml && git commit --amend --no-edit`, rerun this check. A second failure is `PLAN: BLOCKED — every path is a guess: release.yaml drift`.

---

## After Task 10: whole-branch checks

The SDD controller runs these itself, once, after Task 10's commit and before the Phase 4 integration review. For every check: if it fails, the controller dispatches one fix round (a builder with the failing output and this section), then reruns the whole section; a second failure is `PLAN: BLOCKED — every path is a guess: whole-branch <check name>`.

Run: `TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64; PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...`
Expected: last line `0 issues.`

Run: `GOTOOLCHAIN=go1.26.10 govulncheck ./...`
Expected: exit 0 and the line `Your code is affected by 0 vulnerabilities.` (Under go1.27.2 it reports GO-2026-6809 for the local toolchain; that is expected and not this branch.)

Run: `for c in $(git rev-list b2f0353..HEAD); do git cat-file -p $c | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'; done | sort -u`
Expected: exactly one line, `1` (every commit since the design is signed, however many fix-round commits there are).

Run: `git diff --name-only b2f0353 HEAD | grep -vE '^(api/v1alpha1/|cmd/(agent|manager)/main\.go$|config/|internal/|examples/|images/agent/Dockerfile$|bundle/|docs/plans/|README\.md$|CONTRIBUTING\.md$|Makefile$|PROJECT$|go\.mod$|release\.yaml$|grafana-dashboard\.json$|readme/screenshot\.png$)' ; echo done`
Expected: only `done` (no path outside the set this plan touches changed).

Run: `git diff --name-only --diff-filter=A b2f0353 HEAD | grep -vE '^(docs/plans/2026-10-29-m1-trim-implementation\.md|internal/.+_test\.go)$' ; echo done`
Expected: only `done`. The plan adds exactly the plan file, `internal/resources/deployment_test.go`, `internal/resources/service_test.go` and `internal/wireguard/wireguard_test.go`; a fix round may add further `_test.go` files under `internal/`. Any other added path is unplanned.

Run: `grep -rn 'serviceType\|ServiceType\|externalAddress\|PersistentKeepalive\|privateKeyRef\|peerCIDRv6\|wstunnel' config/crd/bases release.yaml ; echo grep-exit=$?`
Expected: only `grep-exit=1`.
