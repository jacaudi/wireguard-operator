# Declarative Deployment and Service Reconciliation — Implementation Plan

**Spec:** docs/designs/2026-08-29-declarative-deployment-reconciliation-design.md (approved 2026-08-30; revised 2026-10-26 after the Fable design gate)
**Issue:** [#2](https://github.com/jacaudi/wireguard-operator/issues/2)
**Base commit:** `be6d4e4`

**Goal:** Replace five duplicated Deployment drift checks and one Service port-drift check with a single Server-Side Apply seam, so every spec field converges without a bespoke check.

**Architecture:** One helper, `applyOwned`, stamps the GVK from the scheme, folds any legacy `Update`-manager ownership into its own apply entry, and issues a Server-Side Apply with a fixed field manager and forced ownership. The Deployment path and both Service paths each become build-then-apply. The apiserver performs the diff; there is deliberately no client-side comparison. Apply errors route through `failDegraded` so a permanent failure surfaces as a Degraded condition instead of silently looping.

**Tech Stack:** Go 1.26 (toolchain go1.26.7 at base, go1.26.10 after Task 8) · `sigs.k8s.io/controller-runtime v0.18.5` · `k8s.io/{api,apimachinery,client-go} v0.30.3` · Ginkgo v1 / Gomega · envtest at Kubernetes 1.30.0

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

**Run contract:** (answered by the repo owner 2026-10-27; recorded verbatim)
- **Integration when done:** (b) push the branch and open a **draft** PR to `main` on `jacaudi/wireguard-operator` with `gh`. Never via the GitHub MCP write tools.
- **Base branch:** `main`.
- **Remote and forge:** `git@github.com:jacaudi/wireguard-operator.git`, using `gh`.
- **Repository owned by the user:** yes (`jacaudi/`), so option (b) is authorized for this branch only.
- **PR details:** draft. Title `fix: reconcile the Deployment and Services declaratively with Server-Side Apply (#2)`. The body is, in order: the plan's **Goal:** line; the handoff's `Rulings I made` list and its `what is complete` commit list (`$MAIN/.superpowers/sdd/<plan-basename>/handoff.md`), with local filesystem paths left out; the `## PR evidence` section of Task 9's report; then `Closes #2`. No labels and no reviewers. The PR is created only in Phase 5, never by a task builder.
- **Before Task 3 is dispatched:** once Task 2's review is clean, the controller commits the design and plan amendment by pathspec, using the command in Task 3 Step 0. That keeps the commit outside every task's review range. The staged Task 2 files are already committed by then. Task 3 Step 0 then finds nothing to commit and continues.
- **Push, SSH auth or `gh` failure at Phase 5 (including a hang on 1Password):** `PLAN: BLOCKED — every path is a guess: finish: push`, or `finish: pr create` if the push succeeded. Never change the remote, never touch credential helpers, never force-push, and never fall back to the GitHub MCP tools.
- **Vulnerability IDs published after 2026-10-28:** a builder that meets a new ID in Task 8 Step 6 or Task 9 Step 2c stops and reports the ID, the module and the fixed version; the controller then rules. Suppose the ID is reachable, its fix is a newer release of `golang.org/x/net`, `google.golang.org/grpc`, a `go.opentelemetry.io/otel*` module or the toolchain line, and after the bump `git diff go.mod | grep -E '^[+-].*(k8s.io|controller-runtime)'` still prints nothing. Then:
  1. Add the module to Task 8 Step 2's `go get`.
  2. State the new Step 3 expectations in the re-dispatch. The module set and diffstat will differ from those printed there; that is intended.
  3. Re-run Task 8 Steps 2–6, and commit as Step 7.
  4. Record it in the report of the task that hit it.

  If the fix needs `k8s.io/*` or `sigs.k8s.io/controller-runtime` to move: `PLAN: BLOCKED — every path is a guess: vulnerability fix needs a pinned module`.
- **Direct push to the base branch:** never. `main` changes only through the merged PR.
- **Branch name:** `worktree-issue-2-declarative-reconciliation` (the existing worktree branch; this overrides the `feat/<name>` default).
- **Sanctioned test command:** the integration suite command in Global Constraints with `-race`, plus `go test ./... -race` for unit tests. The full gate is Task 9 Steps 2a–2d.
- **Sanctioned environment:** the host toolchain (the repo has no `.devcontainer/`).
- **Commit types:** Tasks 4, 5 and 7 use `fix:`, because they change behaviour users can see. The other tasks keep the type given in their commit step.
- **Known vulnerabilities:** fixed in this branch by Task 8 (dependency bumps). The owner chose to bump rather than defer.
- **New dependencies:** allowed only with a present requirement, pinned, and named in the task report. Task 8's version bumps are the sanctioned ones.
- **Boundaries:** 8 hours of wall clock. A dispatch cap of 6 × tasks + 6. 2 hours with no task completing.
- **Worktree after the PR:** keep it until the PR merges. Remove it only on the owner's request.

Steps use checkbox (`- [ ]`) syntax for tracking.

---

## Global Constraints

Every task's requirements implicitly include this section.

- **Go package name is `controllers`, not `controller`.** The directory is `internal/controller/` but every file in it declares `package controllers`. Getting this wrong produces `found packages controllers and controller`.
- **Build tags: TEST files carry `//go:build integration`; production files do not.** In `internal/controller/`, `suite_test.go` and `wireguard_controller_test.go` start with the tag; `wireguard_controller.go` and `wireguardpeer_controller.go` do not. Both directions were verified empirically:
  - A **new test file without the tag** is *included* in the untagged `go test ./...`, where the suite's symbols do not exist, and fails loudly — `undefined: k8sClient`, `[build failed]`. Give every new test file the tag on line 1 followed by a blank line.
  - A **new production file with the tag** disappears from the ordinary build. `apply.go` tagged `integration` leaves `applyOwned` undefined in `go build ./...`, in `task go:build`, in `govulncheck`, and in the `cmd/manager` binary — the seam would exist only in the test build. Give new production files no tag.

  Note the failure is *latent*: with `apply.go` tagged, Task 2's and Task 3's `go build ./...` still pass, because nothing untagged references the new symbols yet. It breaks in Task 4.
- **`Timeout` and `Interval` are consts declared inside the `Describe("wireguard controller", ...)` closure** (`wireguard_controller_test.go:120-121`), not at package scope. A `Describe` in any other file cannot see them and must declare its own.
- **`errors` in `wireguard_controller.go` is `k8s.io/apimachinery/pkg/api/errors`, not stdlib.** Do not call `errors.Is`, `errors.As`, or `errors.Join` on that identifier. If stdlib errors are needed, import it as `stderrors "errors"`.
- **Conventional commit subjects are mandatory.** release-please parses them; a type-less subject contributes no changelog entry and no version bump. Use exactly the type given in each task's commit step: Tasks 4, 5 and 7 are `fix:` (owner ruling, Run contract), Task 8 is `fix(deps):`, and the others are `refactor:`, `test:` or `docs:` as written. Design §10.1 item 3's `refactor:` is superseded on commit type.
- **Do not use `feat!:` or a `BREAKING CHANGE:` footer anywhere in this work.** It would bump 2.11.0 → 3.0.0.
- **Never claim a step passed without running it and reading the output.**
- **Run the integration suite with:**
  ```bash
  KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1
  ```
  Add `-race` before the final commit of each task; CI runs `-race`.
- **Full local gate before the final task's commit:** `task ci`, then the explicit lint, tidy, vuln and test steps in Task 9 Steps 2a–2d (`task ci` stops at its first failing stage, and on this machine that is lint).
- **Do not touch:** `updateWireguardPeers`, the Secret block, the ConfigMap block, or the readiness gates. All are out of scope by design decision.
- **`k8sClient` (used by tests) is a direct, non-cached client.** The reconciler's client is cache-backed. Assertions must use `k8sClient`.
- **Test writes create co-ownership (design §7.3, binding on every spec).** A test that writes an operator-owned object (Deployment `vpn-dep`, Services `vpn-svc` and `vpn-metrics-svc`) makes the test client a co-owner of the fields it changed, and the apiserver never prunes a co-owned field. Such a write can therefore hide exactly the removal a spec is trying to prove. Two rules follow. Tamper with an operator-owned object only on a field the operator will change back (a conflict, which the forced apply transfers). Never `Create` an operator-owned object, except in Task 7's adoption specs, which must then exercise a removal or an allocation check. Drive removals through the `Wireguard` spec only.
- **The production legacy field manager is the literal `"manager"`.** Do not put `controllers.test` (envtest's User-Agent-derived name) in production code. Specs that need a legacy object seed it with `k8sClient.Create(ctx, obj, client.FieldOwner("manager"))`.
- **Work only in the existing worktree** `/Users/acaudill/Projects/github/wireguard-operator/.claude/worktrees/issue-2-declarative-reconciliation` on branch `worktree-issue-2-declarative-reconciliation`; never create another worktree.
- **Every commit is signed** (`commit.gpgsign=true`, `gpg.format=ssh`, via 1Password). Never pass `--no-gpg-sign` or override `commit.gpgsign`. If `git commit` fails with a signing error, or hangs waiting on 1Password, stop and report BLOCKED (commit signing unavailable) with the files left staged; the controller, not the builder, writes `PLAN: BLOCKED — every path is a guess: commit signing unavailable` to the ledger. After every commit, confirm the signature is present:
  ```bash
  git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'
  ```
  Expected: `1`. Do **not** gate on `git log -1 --format=%G?`: on this machine it prints `N` for every signed commit, with `error: gpg.ssh.allowedSignersFile needs to be configured`, because no allowed-signers file is set up to *verify* SSH signatures (measured 2026-10-27 on `e7d4579`, which carries an SSH signature). Do not configure one; that is out of scope.
- **Lint is gated on the printed `0 issues.` line, never on the exit status.** The RTK hook rewrites `golangci-lint` and swallows its exit code. Read the output, or run it as `rtk proxy golangci-lint …`.
- **Any scratch-worktree measurement goes under `/private/tmp`, never `/tmp`.** Under `/tmp` the lint ratchet falsely reports `0 issues.`
- **Ledger entries written before 2026-10-26 use the pre-amendment numbering:** old Task 3→4, 4→5, 5→6, 6→7, 7→9.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `internal/controller/apply.go` *(new)* | The `applyOwned` seam (including the legacy field-manager migration), `FieldManager`, `legacyFieldManagers`, and `failDegraded`. Nothing else. | 2, 3 |
| `internal/controller/apply_test.go` *(new)* | envtest coverage of the seam in isolation, including the migration. | 2, 3 |
| `internal/controller/wireguard_controller.go` | Deployment and Service paths become build-then-apply; `updateStatus` clears `Degraded` alongside `Ready=True`. | 4, 5, 7 |
| `internal/controller/wireguard_controller_test.go` | Port-identity fixes at three sites; new convergence specs. | 1, 4, 5, 7 |
| `internal/controller/churn_test.go` *(new)* | Counting client and the no-churn spec. Isolated because it needs its own namespace and its own reconciler wiring. | 6 |
| `internal/controller/suite_test.go` | Wraps the Wireguard reconciler's client in the counting client. | 6 |
| `go.mod`, `go.sum` | Dependency and toolchain bumps that clear the reachable vulnerabilities. No Go source changes. | 8 |
| `docs/ROADMAP.md` | Stale line numbers in § Phase 0.2. | 9 |
| `.golangci.yml` | Stale complexity figures and the "replaces wholesale" claim in the ratchet comment (`:89-92`). | 9 |

---

## Task 1: Fix all three Service port-mangle sites (blocking prerequisite)

> **Renumbered 2026-10-26 (done, `e7d4579`):** "Task 4" below now means Task 5 (Service paths). This commit left a gofmt defect at `wireguard_controller_test.go:62`, which Task 4 Step 5 fixes.

**Why first:** `reconcileServiceWithClusterIP` replaces the Service port list with an entry that has no `Name` and no `Protocol`. The apiserver defaults `Protocol` to `TCP`. Because `ServiceSpec.Ports` is `+listType=map` keyed on **both** `port` and `protocol` (`k8s.io/api@v0.30.3/core/v1/types.go:5130-5133`), the operator's `{name:"wireguard", protocol:UDP, port:51820}` will not replace it once Task 4 lands — it will be added *alongside* it. A multi-port Service requires every port to be named, so the apply then fails permanently with `spec.ports[0].name: Required value` and the operator never converges.

Seven specs call that helper, and two more specs are broken by the other two sites below. Fixing all of them first means Task 4 does not have to debug a red suite.

The sibling helper `reconcileServiceWithTypeNodePort` (line 45) already solves this and even carries the comment *"preserving existing port names and protocols"*. Copy its pattern.

**There are three distinct mangle sites, not one.** All three must be fixed in this task; fixing only the helper leaves two live failures:

| site | what it does | resulting apply failure |
|---|---|---|
| `:74` `reconcileServiceWithClusterIP` | replaces the list with an unnamed, protocol-less port | `spec.ports[0].name: Required value` |
| `:371` `reconcileServiceWithTypeNodePort(serviceKey, expectedNodePort, 5182)` | sets `Port` to **5182**, changing the merge key from `(51820, UDP)` to `(5182, UDP)` while keeping `Name: "wireguard"` | `spec.ports[1].name: Duplicate value: "wireguard"` |
| `:838` inline in the DNS search-domain spec | same defect as the helper, copy-pasted rather than called | `spec.ports[0].name: Required value` |

`5182` is a typo for `51820` — verified that the value is referenced nowhere else in the file and asserted by nothing, so correcting it changes no assertion.

The `:838` site is the DRY violation underneath all of this: *"make the ClusterIP Service look ready"* is one piece of knowledge expressed in two places, and only one of them got the earlier fix.

**A note on the design's preference.** Design §7.2 named *"delete the helper and its call sites"* as preferred, since the ClusterIP readiness gate only checks `len(Ports) > 0`, which the operator's own apply now guarantees. This plan takes the fallback — preserve identity in place — because it touches three lines instead of eight call sites, and because identity-preservation is already the established pattern in this file (`reconcileServiceWithTypeNodePort`). Deleting the mutations entirely remains a reasonable follow-up.

**Files:**
- Modify: `internal/controller/wireguard_controller_test.go` — `:74-87`, `:371`, `:838`

**Interfaces:**
- Consumes: nothing
- Produces: `reconcileServiceWithClusterIP(svcKey client.ObjectKey, port int32) error` — signature unchanged; only the body changes.

- [ ] **Step 1: Confirm the current suite is green before touching anything**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1
```

Expected: `ok github.com/jacaudi/wireguard-operator/internal/controller`. If this is already red, stop and report — do not proceed.

- [ ] **Step 2: Replace the helper body**

In `internal/controller/wireguard_controller_test.go`, replace this:

```go
	svc.Spec.Ports = []corev1.ServicePort{{
		Port:       port,
		TargetPort: intstr.FromInt32(port),
	}}
	return k8sClient.Update(context.Background(), svc)
```

with this:

```go
	// Mutate in place, preserving existing port names and protocols. Service ports are an
	// associative list keyed on BOTH port and protocol, so replacing the slice with an entry
	// that has no name and no protocol creates a second, unnamed port rather than replacing
	// the operator's — and a multi-port Service requires every port to be named.
	// Mirrors reconcileServiceWithTypeNodePort above.
	if len(svc.Spec.Ports) > 0 {
		svc.Spec.Ports[0].Port = port
		svc.Spec.Ports[0].TargetPort = intstr.FromInt32(port)
	}
	return k8sClient.Update(context.Background(), svc)
```

- [ ] **Step 3: Fix the NodePort call site's port typo**

At `internal/controller/wireguard_controller_test.go:371`, change:

```go
			Expect(reconcileServiceWithTypeNodePort(serviceKey, expectedNodePort, 5182)).Should(Succeed())
```

to:

```go
			Expect(reconcileServiceWithTypeNodePort(serviceKey, expectedNodePort, 51820)).Should(Succeed())
```

`5182` changes the port's merge key, so the operator's apply adds `(51820, UDP)` alongside it and the Service ends up with two ports both named `wireguard`. Nothing asserts `5182`.

- [ ] **Step 4: Fix the inline mangle in the DNS search-domain spec**

At `internal/controller/wireguard_controller_test.go:838`, inside the `Eventually` that makes the Service look ready, change:

```go
				svc.Spec.Ports = []corev1.ServicePort{{Port: 51820, TargetPort: intstr.FromInt(51820)}}
```

to:

```go
				// Preserve port name and protocol; see reconcileServiceWithClusterIP.
				if len(svc.Spec.Ports) > 0 {
					svc.Spec.Ports[0].Port = 51820
					svc.Spec.Ports[0].TargetPort = intstr.FromInt(51820)
				}
```

- [ ] **Step 5: Confirm no other port-list replacement remains**

```bash
grep -n "Spec.Ports = \[\]corev1.ServicePort" internal/controller/wireguard_controller_test.go
```

Expected: no output. If any site remains, apply the same in-place pattern before continuing.

- [ ] **Step 6: Verify the suite is still green**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race
```

Expected: PASS. This change is behaviour-preserving today — the ClusterIP readiness gate only checks `len(svcFound.Spec.Ports) > 0`, which both forms satisfy.

- [ ] **Step 7: Commit**

```bash
git add internal/controller/wireguard_controller_test.go
git commit -m "test: preserve port identity at all three Service port mangle sites

Service ports are an associative list keyed on BOTH port and protocol, so any
test mutation that drops the name and protocol -- or changes the port number --
adds a second entry rather than replacing the operator's. Harmless today,
because the drift check only compares len(Ports) and Ports[0].Port, but it
wedges a Server-Side Apply reconciler permanently.

Three sites, three failure modes:
  - reconcileServiceWithClusterIP dropped name and protocol
    -> spec.ports[0].name: Required value
  - the NodePort call site passed 5182, a typo for 51820, changing the merge key
    -> spec.ports[1].name: Duplicate value: "wireguard"
  - the DNS search-domain spec inlined a copy of the first defect
    -> spec.ports[0].name: Required value

All three now mutate in place, matching reconcileServiceWithTypeNodePort.

Refs #2"
```

Then confirm the commit is signed (Global Constraints):

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. A missing signature means signing silently failed: stop and report BLOCKED (commit signing unavailable).

---

## Task 2: The `applyOwned` seam

> **Renumbered 2026-10-26 (staged, not committed):** "Task 3" below now means Task 4 (Deployment path). M1 error wrapping and the legacy field-manager migration land in the new Task 3; the staged code is final for this task. The staged `apply_test.go` differs from Step 1's block in one place (Self-Review, *Disclosed divergences* item 7).

**Files:**
- Create: `internal/controller/apply.go`
- Create: `internal/controller/apply_test.go`

**Interfaces:**
- Consumes: `WireguardReconciler` (embeds `client.Client`, has `Scheme *runtime.Scheme`), `updateStatus(ctx, req, cond) error`, `ConditionDegraded` — all already exist in `wireguard_controller.go`.
- Produces:
  - `const FieldManager = "wireguard-operator"`
  - `func (r *WireguardReconciler) applyOwned(ctx context.Context, obj client.Object) error`
  - `func (r *WireguardReconciler) failDegraded(ctx context.Context, req ctrl.Request, err error, reason, msg string) (ctrl.Result, error)`

**Note on a deliberate simplification from the design:** the design sketched `failDegraded` using `errors.Join`. That identifier is bound to apimachinery's errors package in this file, and importing stdlib errors under an alias to combine two errors is not worth the noise. Instead, a status-update failure is logged and the original error returned — the original error is what the caller needs to see, and the status failure is already retried on the next reconcile.

- [ ] **Step 1: Write the failing test**

Create `internal/controller/apply_test.go`:

```go
//go:build integration

package controllers

import (
	"context"

	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("applyOwned", func() {
	// Timeout and Interval in wireguard_controller_test.go are declared inside its Describe
	// closure and are not visible here. Declare our own.
	const (
		applyNS       = "default"
		applyTimeout  = time.Second * 10
		applyInterval = time.Millisecond * 250
	)

	// newTestReconciler builds a reconciler wired to the direct test client. Its builders are
	// normally initialised by SetupWithManager, which we do not call here, so nothing in this
	// spec may touch r.deploymentBuilder and friends.
	newTestReconciler := func() *WireguardReconciler {
		return &WireguardReconciler{
			Client:               k8sClient,
			Scheme:               scheme.Scheme,
			AgentImage:           wgTestImage,
			AgentImagePullPolicy: corev1.PullIfNotPresent,
		}
	}

	// A minimal Deployment that does not collide with the operator-managed "vpn-dep".
	desired := func() *appsv1.Deployment {
		labels := map[string]string{"app": "apply-seam-probe"}
		replicas := int32(1)
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "apply-seam-probe", Namespace: applyNS},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: "probe", Image: "registry.invalid/probe:v1",
					}}},
				},
			},
		}
	}

	key := types.NamespacedName{Name: "apply-seam-probe", Namespace: applyNS}

	AfterEach(func() {
		dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}}
		_ = k8sClient.Delete(context.Background(), dep)
	})

	It("creates the object, derives the GVK, and owns it under FieldManager", func() {
		r := newTestReconciler()
		obj := desired()

		Expect(r.applyOwned(context.Background(), obj)).Should(Succeed())

		// The GVK must have been stamped on -- SSA rejects a body without apiVersion/kind.
		Expect(obj.GroupVersionKind().Kind).Should(Equal("Deployment"))
		Expect(obj.GroupVersionKind().GroupVersion().String()).Should(Equal("apps/v1"))

		// The apply response is decoded back into obj, so it carries server state.
		Expect(obj.UID).ShouldNot(BeEmpty())
		Expect(obj.ResourceVersion).ShouldNot(BeEmpty())

		live := &appsv1.Deployment{}
		Expect(k8sClient.Get(context.Background(), key, live)).Should(Succeed())
		Expect(obj.ResourceVersion).Should(Equal(live.ResourceVersion))

		var managers []string
		for _, mf := range live.ManagedFields {
			managers = append(managers, mf.Manager)
		}
		Expect(managers).Should(ContainElement(FieldManager))
	})

	It("does not churn resourceVersion when applied repeatedly with identical content", func() {
		r := newTestReconciler()
		Expect(r.applyOwned(context.Background(), desired())).Should(Succeed())

		first := &appsv1.Deployment{}
		Expect(k8sClient.Get(context.Background(), key, first)).Should(Succeed())

		for i := range 4 {
			Expect(r.applyOwned(context.Background(), desired())).Should(Succeed())
			now := &appsv1.Deployment{}
			Expect(k8sClient.Get(context.Background(), key, now)).Should(Succeed())
			Expect(now.ResourceVersion).Should(Equal(first.ResourceVersion),
				"apply %d persisted a write when nothing changed", i+1)
			Expect(now.Generation).Should(Equal(first.Generation))
		}
	})

	It("converges a field changed by another writer", func() {
		r := newTestReconciler()
		Expect(r.applyOwned(context.Background(), desired())).Should(Succeed())

		tampered := &appsv1.Deployment{}
		Expect(k8sClient.Get(context.Background(), key, tampered)).Should(Succeed())
		tampered.Spec.Template.Spec.Containers[0].Image = "registry.invalid/tampered:latest"
		Expect(k8sClient.Update(context.Background(), tampered, client.FieldOwner("someone-else"))).Should(Succeed())

		Expect(r.applyOwned(context.Background(), desired())).Should(Succeed())

		live := &appsv1.Deployment{}
		Expect(k8sClient.Get(context.Background(), key, live)).Should(Succeed())
		Expect(live.Spec.Template.Spec.Containers[0].Image).Should(Equal("registry.invalid/probe:v1"))
	})

	It("records a Degraded condition when failDegraded is called", func() {
		r := newTestReconciler()

		nn := types.NamespacedName{Name: "apply-degraded", Namespace: applyNS}
		wg := &v1alpha1.Wireguard{ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace}}
		Expect(k8sClient.Create(context.Background(), wg)).Should(Succeed())
		defer func() {
			// The manager reconciles this object, so it also creates children. Clean them up:
			// BeforeEach in wireguard_controller_test.go does not delete Deployments, and
			// envtest runs no garbage collector, so anything left here leaks for the whole run.
			_ = k8sClient.Delete(context.Background(), wg)
			_ = k8sClient.Delete(context.Background(), &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: nn.Name + "-dep", Namespace: applyNS}})
			for _, svcName := range []string{nn.Name + "-svc", nn.Name + "-metrics-svc"} {
				_ = k8sClient.Delete(context.Background(), &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: applyNS}})
			}
		}()

		_, err := r.failDegraded(context.Background(), ctrlRequestFor(nn), errProbe, "ProbeFailed", "probe failed")
		Expect(err).Should(MatchError(errProbe))

		// Assert on the CONDITION, not on Status.Status.
		//
		// The manager's reconciler is watching this object and writes its own conditions
		// concurrently. updateStatus derives the summary field from whichever condition it just
		// wrote, so Status.Status flips to "pending" the moment the manager reconciles -- there
		// is no name the manager will not pick up, and asserting the summary field is a race.
		// Conditions are merged by type, so the Degraded condition survives alongside the
		// manager's Progressing one.
		Eventually(func() bool {
			live := &v1alpha1.Wireguard{}
			if err := k8sClient.Get(context.Background(), nn, live); err != nil {
				return false
			}
			for _, c := range live.Status.Conditions {
				if c.Type == ConditionDegraded && c.Reason == "ProbeFailed" &&
					c.Status == metav1.ConditionTrue {
					return true
				}
			}
			return false
		}, applyTimeout, applyInterval).Should(BeTrue(),
			"expected a Degraded condition with reason ProbeFailed")
	})
})
```

Add these two package-level helpers at the bottom of the same file:

```go
var errProbe = stderrors.New("probe error")

func ctrlRequestFor(nn types.NamespacedName) ctrl.Request {
	return ctrl.Request{NamespacedName: nn}
}
```

The file's complete import block — do not merge these by hand, use exactly this:

```go
import (
	"context"
	stderrors "errors"
	"time"

	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -run TestAPIs 2>&1 | tail -20
```

Expected: a **compile failure** — `undefined: FieldManager`, `r.applyOwned undefined`, `r.failDegraded undefined`. That is the correct failing state.

- [ ] **Step 3: Write the implementation**

Create `internal/controller/apply.go`. **No build tag** — this is production code, and `wireguard_controller.go`, which calls `applyOwned` from Task 3 onward, is untagged:

```go
// Package controllers implements the Wireguard and WireguardPeer reconcilers.
//
// golangci-lint's revive `package-comments` rule fires on every line of a new file under the
// `new-from-merge-base` ratchet, and no existing file in this package carries a package comment.
// Putting it here, on the new file, satisfies the rule without touching unrelated files.
package controllers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// FieldManager identifies this operator's Server-Side Apply field manager. It is part of the
// on-cluster contract: changing it orphans field ownership on every existing object and leaves
// stale entries in managedFields. Not a tunable.
const FieldManager = "wireguard-operator"

// applyOwned renders an operator-owned object into the cluster with Server-Side Apply. The
// apiserver performs the diff; there is deliberately no client-side comparison.
//
// What the operator ends up owning is what json.Marshal of the built object emits, which is not
// the same as "the fields the builder sets", in either direction. Zero-valued structs are emitted
// despite omitempty -- so the operator owns spec.strategy, the agent container's resources, and
// the pod template's creationTimestamp, none of which any builder sets. Zero-valued scalars with
// omitempty are dropped -- so a nodePort of 0 is not owned, which is what lets the apiserver keep
// its own allocation.
//
// ForceOwnership is unconditional. A conflict is only raised when the applied value differs from
// a value owned by another manager, so the first apply over an object previously written by
// Create/Update succeeds without it -- but the first apply that actually changes such a field
// would fail, and the operator would wedge. Forcing is also what makes the operator converge an
// object a human edited with kubectl, which is the point of this change.
//
// SSA requires apiVersion and kind in the request body, and a typed object's TypeMeta is empty
// after construction, so the GVK is derived from the scheme and stamped on before the call.
// Without this the apiserver rejects the patch with "invalid object type: /, Kind=".
func (r *WireguardReconciler) applyOwned(ctx context.Context, obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, r.Scheme)
	if err != nil {
		return err
	}
	obj.GetObjectKind().SetGroupVersionKind(gvk)
	return r.Patch(ctx, obj, client.Apply,
		client.FieldOwner(FieldManager), client.ForceOwnership)
}

// failDegraded records a Degraded condition and returns the original error for requeue.
//
// Some apply failures are permanent rather than transient -- a Service port colliding with a
// foreign writer's entry, or spec.nodePort set on a ClusterIP Service. Without a status
// condition the operator retries forever with a stale Status and nothing user-visible.
//
// A failure to write the status is logged rather than combined into the returned error: the
// caller needs the original error, and the status write is retried on the next reconcile.
func (r *WireguardReconciler) failDegraded(
	ctx context.Context, req ctrl.Request, err error, reason, msg string,
) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	log.Error(err, msg)

	if serr := r.updateStatus(ctx, req, metav1.Condition{
		Type:    ConditionDegraded,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: err.Error(),
	}); serr != nil {
		log.Error(serr, "failed to record Degraded condition", "reason", reason)
	}
	return ctrl.Result{}, err
}
```

> **Note on the build tag:** `apply.go` must have **no** build tag. Only the two `_test.go` files in this package are tagged `integration`; `wireguard_controller.go` and `wireguardpeer_controller.go` are not. Tagging `apply.go` would leave `applyOwned` undefined in the ordinary build, breaking `go build ./...` and the manager binary. Step 4 verifies both the tagged and the untagged build.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race
go build ./...
go vet ./internal/controller/
```

Expected: PASS, and both builds clean. `go build ./...` is the one that catches a stray
`//go:build integration` on `apply.go` — with the tag, `applyOwned` vanishes from the ordinary
build and the manager binary fails to compile once Task 3 calls it.

If the `applyOwned` create fails with `invalid object type: /, Kind=`, the GVK stamping is not working — check that `r.Scheme` is non-nil and that `scheme.Scheme` has `appsv1` registered.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/apply.go internal/controller/apply_test.go
git commit -m "refactor: add applyOwned Server-Side Apply seam

One helper that owns the field-manager identity and the GVK requirement, so
every operator-owned object converges through a single path and no future spec
field needs its own drift check.

Also adds failDegraded, so a permanent apply failure surfaces as a Degraded
condition instead of looping silently with a stale status.

Refs #2"
```

Then confirm the commit is signed (Global Constraints):

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. A missing signature means signing silently failed: stop and report BLOCKED (commit signing unavailable).

---

## Task 3: Legacy field-manager migration in applyOwned
Kind: rewrite

**Why:** every Deployment and Service that exists before this change was written by `Create`/`Update`, so an `Update`-operation manager owns all of its fields. In production that manager is called `manager`: the image runs `/manager` (`images/manager/Dockerfile`), `cmd/manager/main.go` sets no custom User-Agent, and the apiserver takes the User-Agent segment before `/`. A forced apply transfers ownership **only of the fields whose value differs** (`sigs.k8s.io/structured-merge-diff/v4@v4.4.1/merge/update.go:108`). The legacy manager keeps every field the operator renders unchanged, and the apiserver never prunes a field another manager still owns (`update.go:245-247`, `:305-307`). On every pre-existing installation, turning the wstunnel sidecar off, deleting a `serviceAnnotations` key, setting `tolerations` to null or dropping a dualMode port would therefore never take effect. Design §6.1.

**The fix lives inside `applyOwned`**, the one seam all three callers already use, so no caller learns that legacy objects exist. Before the forced apply, `applyOwned` reads the live object through the reconciler's client (cache-backed in production) into `obj.DeepCopyObject()`:

- NotFound → skip; the apply creates the object, owned by `FieldManager` alone.
- Any other error → return it, wrapped.
- Found → `csaupgrade.UpgradeManagedFieldsPatch(live, legacyFieldManagers, FieldManager)`. A non-nil patch is sent as `client.RawPatch(types.JSONPatchType, patch)`, error wrapped. A nil patch means nothing is left to migrate, and nothing is sent.

Then the existing forced apply runs unchanged. Cost: one extra write per legacy object, once.

**This task also carries M1:** the bare `GVKForObject` and `Patch` returns in the staged `applyOwned` get context via `%w`.

**Verified API (do not re-derive):** `k8s.io/client-go@v0.30.3/util/csaupgrade/upgrade.go:122`:

```go
func UpgradeManagedFieldsPatch(obj runtime.Object, csaManagerNames sets.Set[string], ssaManagerName string, opts ...Option) ([]byte, error)
```

It returns `nil, nil` when no `Update` entry named in `csaManagerNames` exists (`:147-151`, `:216-220`). Its patch replaces both `/metadata/managedFields` and `/metadata/resourceVersion` (`:157-171`), so a stale cache read fails with a 409 Conflict instead of overwriting newer managedFields. Both `k8s.io/client-go` and `k8s.io/apimachinery` are already direct requirements at `v0.30.3`, so **`go.mod` and `go.sum` do not change**. `go mod tidy` was measured clean after this task.

**Files:**
- Modify: `internal/controller/apply.go` (rewritten above `failDegraded`): package comment, imports, a new `legacyFieldManagers` var, and the `applyOwned` doc comment and body. `failDegraded` is untouched.
- Modify: `internal/controller/apply_test.go`: two new specs and two imports.

**Interfaces:**
- Consumes: `applyOwned`, `FieldManager`, `failDegraded` and the spec helpers `newTestReconciler`, `desired`, `key` (Task 2, staged).
- Produces:
  - `var legacyFieldManagers = sets.New("manager")` (package-level, unexported)
  - `func (r *WireguardReconciler) applyOwned(ctx context.Context, obj client.Object) error`, with the same signature as before. Its errors now carry context and still unwrap to the apiserver's `*StatusError` (`apierrors.IsInvalid`, `apierrors.IsConflict` and the rest keep working).
  - Write behaviour later tasks rely on: on a legacy object, `applyOwned` issues **two** writes (one JSON patch, then the apply) the first time, and **one** apply ever after. On an object it created, it only ever issues the apply. Task 6's counting client tells the two apart by patch type.

- [ ] **Step 0: Confirm Task 2 is committed and the amendment is committed**

Task 2's files were staged, not committed, when this plan was amended. This task's commit must not swallow them.

Run: `git log --format=%s -- internal/controller/apply.go | grep -c '^refactor: add applyOwned Server-Side Apply seam$'`
Expected: `1`. Newer commits for Task 2's review fixes, or for this task, may sit above it; only this subject has to be present.

If it prints `0`, run `git log --format=%s -- internal/controller/apply.go internal/controller/apply_test.go`. If that output is empty, Task 2 is still uncommitted: run Task 2 Step 5's commit (both files are already staged), then its signature check, then re-run this step. If that commit fails, stop and report BLOCKED (commit signing unavailable). If the output is not empty, stop and report BLOCKED: Task 2's files went into the wrong commit.

The controller commits the design and plan amendment before dispatching this task (Run contract, *Before Task 3 is dispatched*). Confirm it:

Run: `git status --short --untracked-files=no`
Expected: no output.

If the design or plan still shows ` M`, commit them by pathspec, so that nothing else staged goes with them:

Run: `git commit -m "docs: amend issue #2 design and plan after review gates" -- docs/designs/2026-08-29-declarative-deployment-reconciliation-design.md docs/plans/2026-08-30-declarative-deployment-reconciliation-implementation.md && git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. If the commit fails with a signing error or hangs waiting on 1Password, stop and report BLOCKED (commit signing unavailable). The design and plan stay unstaged.

**A/B note:** `Kind: rewrite` refers to `apply.go`, but `apply.go` and `apply_test.go` do not exist at the merge base. Their comparator is the `head7` of the ledger line `Task 2: complete (commits <base7>..<head7>, …)`, used as `git show <head7>:internal/controller/apply.go`, not the merge base.

- [ ] **Step 1: Write the failing specs**

In `internal/controller/apply_test.go`, insert these two specs inside `Describe("applyOwned", ...)`, directly after the closing `})` of `It("converges a field changed by another writer", ...)` and before `It("records a Degraded condition when failDegraded is called", ...)`:

```go
	It("migrates a legacy Update-owned object so a field it stops rendering is removed", func() {
		ctx := context.Background()

		// withSidecar renders the probe plus a second container. Seeded under "manager" -- the
		// production binary's field manager before this change; the literal pins that on-cluster
		// contract -- it is the pre-SSA shape: an object written by Create, owned by an Update entry.
		withSidecar := func() *appsv1.Deployment {
			dep := desired()
			dep.Spec.Template.Spec.Containers = append(dep.Spec.Template.Spec.Containers,
				corev1.Container{Name: "legacy-sidecar", Image: "registry.invalid/sidecar:v1"})
			return dep
		}
		Expect(k8sClient.Create(ctx, withSidecar(), client.FieldOwner("manager"))).Should(Succeed())
		seeded := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, key, seeded)).Should(Succeed())

		// Adopt with identical content. The migration rewrites ownership only: the stored spec
		// is untouched, and no Update entry is left behind -- not "manager", and not one for the
		// client that sent the migration patch.
		r := newTestReconciler()
		Expect(r.applyOwned(ctx, withSidecar())).Should(Succeed())

		adopted := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, key, adopted)).Should(Succeed())
		Expect(adopted.Generation).Should(Equal(seeded.Generation), "adoption must not change the spec")
		var managers []string
		for _, mf := range adopted.ManagedFields {
			managers = append(managers, mf.Manager)
		}
		Expect(managers).Should(ConsistOf(FieldManager))

		// Stop rendering the sidecar. Only an exclusive owner can prune it.
		Expect(r.applyOwned(ctx, desired())).Should(Succeed())

		live := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, key, live)).Should(Succeed())
		var containers []string
		for _, c := range live.Spec.Template.Spec.Containers {
			containers = append(containers, c.Name)
		}
		Expect(containers).Should(Equal([]string{"probe"}),
			"a container the legacy manager co-owned must be pruned once ownership migrates")

		// The migration is one write, once: re-applying identical content persists nothing.
		Expect(r.applyOwned(ctx, desired())).Should(Succeed())
		again := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, key, again)).Should(Succeed())
		Expect(again.ResourceVersion).Should(Equal(live.ResourceVersion))
	})

	It("wraps its errors with the object's identity and keeps the API status inspectable", func() {
		ctx := context.Background()

		unregistered := newTestReconciler()
		unregistered.Scheme = runtime.NewScheme()
		Expect(unregistered.applyOwned(ctx, desired())).Should(
			MatchError(ContainSubstring("resolve GVK for *v1.Deployment")))

		invalid := desired()
		invalid.Spec.Template.Spec.Containers = nil
		err := newTestReconciler().applyOwned(ctx, invalid)
		Expect(err).Should(MatchError(ContainSubstring("apply Deployment default/apply-seam-probe")))
		Expect(apierrors.IsInvalid(err)).Should(BeTrue(), "the apiserver's status must survive wrapping: %v", err)
	})
```

Add two imports to the same file. The complete import block becomes:

```go
import (
	"context"
	stderrors "errors"
	"time"

	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)
```

The literal `"manager"` in the first spec is deliberate. It pins the on-cluster contract rather than echoing the production variable, so changing `legacyFieldManagers` by mistake turns this spec red.

- [ ] **Step 2: Run the specs to verify they fail**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -v -ginkgo.focus=applyOwned 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | grep -E '^\[Fail\]|Ran [0-9]+ of|Passed \||<\[\]string|resolve GVK'
```

Expected (measured 2026-10-26 against the staged Task 2 code):

```
      <[]string | len:2, cap:2>: ["wireguard-operator", "manager"]
      <[]string | len:1, cap:1>: ["wireguard-operator"]
      <[]string | len:1, cap:1>: ["manager"]
          Substr: "resolve GVK for *v1.Deployment",
[Fail] applyOwned [It] migrates a legacy Update-owned object so a field it stops rendering is removed
[Fail] applyOwned [It] wraps its errors with the object's identity and keeps the API status inspectable
Ran 6 of 29 Specs in ...
FAIL! -- 4 Passed | 2 Failed | 0 Pending | 23 Skipped
```

The `[Fail]` lines can appear in any order (Ginkgo v1 shuffles top-level containers on each seed); match the set, not the order.


The three `<[]string` lines are Gomega's `ConsistOf` report: the actual managers, the expected set, and the extra element. (Re-measured 2026-10-27; the `[Fail]` lines carry a trailing space in the raw output.)

The first failure is B1 itself: after an identical-content apply, `manager` still co-owns everything. The second is M1: the error is a bare `*runtime.notRegisteredErr` with no context. The four Task 2 specs must stay green. If either new spec fails on anything else (a compile error, or `AlreadyExists` on the seed), fix the spec, not the assertion.

- [ ] **Step 3: Implement the migration and the error wrapping**

In `internal/controller/apply.go`, replace **everything above** the line `// failDegraded records a Degraded condition and returns the original error for requeue.` with the block below. `failDegraded` and its doc comment stay exactly as they are. The file keeps **no build tag**.

```go
// Package controllers implements the Wireguard and WireguardPeer reconcilers.
package controllers

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/csaupgrade"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// FieldManager identifies this operator's Server-Side Apply field manager. It is part of the
// on-cluster contract: changing it orphans field ownership on every existing object and leaves
// stale entries in managedFields. Not a tunable.
const FieldManager = "wireguard-operator"

// legacyFieldManagers names the Update-operation field manager that owns every field of an
// object written by this operator before it used Server-Side Apply. The apiserver takes the
// manager name from the client's User-Agent, whose first segment is the binary name: the image
// runs /manager (images/manager/Dockerfile) and cmd/manager sets no custom User-Agent, so it is
// "manager". Part of the on-cluster contract, like FieldManager. Objects last written by a
// differently named binary, such as a `go run` dev build, are not migrated.
var legacyFieldManagers = sets.New("manager")

// applyOwned renders an operator-owned object into the cluster with Server-Side Apply. The
// apiserver performs the diff; there is deliberately no client-side comparison. obj is
// overwritten with the server's response, so each call needs a freshly built object.
//
// What the operator ends up owning is what json.Marshal of the built object emits, which is not
// the same as "the fields the builder sets", in either direction. Zero-valued structs are emitted
// despite omitempty -- so the operator owns spec.strategy, the agent container's resources, and
// the pod template's creationTimestamp, none of which any builder sets. Zero-valued scalars with
// omitempty are dropped -- so a nodePort of 0 is not owned, which is what lets the apiserver keep
// its own allocation.
//
// ForceOwnership is unconditional. A conflict is only raised when the applied value differs from
// a value owned by another manager, so the first apply over an object previously written by
// Create/Update succeeds without it -- but the first apply that actually changes such a field
// would fail, and the operator would wedge. Forcing is also what makes the operator converge an
// object a human edited with kubectl, which is the point of this change.
//
// Forcing alone does not finish adoption. It transfers only the fields whose value differs, so
// the legacy manager keeps every field the operator renders unchanged, and the apiserver never
// prunes a field another manager still owns: stop rendering one -- the wstunnel sidecar, a
// toleration, a service annotation -- and it stays. So before applying to an object that already
// exists, the legacy manager's fields are merged into FieldManager's apply entry
// (csaupgrade.UpgradeManagedFieldsPatch). That costs one extra write per legacy object, once;
// on every later call the computed patch is nil and nothing is sent. The patch pins the
// resourceVersion that was read, so a stale cache read fails with a conflict and is retried on
// the next reconcile rather than overwriting newer managedFields.
//
// SSA requires apiVersion and kind in the request body, and a typed object's TypeMeta is empty
// after construction, so the GVK is derived from the scheme and stamped on before the call.
// Without this the apiserver rejects the patch with "invalid object type: /, Kind=".
func (r *WireguardReconciler) applyOwned(ctx context.Context, obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, r.Scheme)
	if err != nil {
		return fmt.Errorf("resolve GVK for %T: %w", obj, err)
	}
	obj.GetObjectKind().SetGroupVersionKind(gvk)
	key := client.ObjectKeyFromObject(obj)

	// Only managedFields and resourceVersion of the live object are consumed.
	live := obj.DeepCopyObject().(client.Object)
	switch err := r.Get(ctx, key, live); {
	case apierrors.IsNotFound(err):
		// Nothing to migrate: the apply below creates the object, owned by FieldManager alone.
	case err != nil:
		return fmt.Errorf("read %s %s for field-manager migration: %w", gvk.Kind, key, err)
	default:
		patch, err := csaupgrade.UpgradeManagedFieldsPatch(live, legacyFieldManagers, FieldManager)
		if err != nil {
			return fmt.Errorf("compute field-manager migration for %s %s: %w", gvk.Kind, key, err)
		}
		if patch != nil {
			if err := r.Patch(ctx, live, client.RawPatch(types.JSONPatchType, patch)); err != nil {
				return fmt.Errorf("migrate field managers on %s %s: %w", gvk.Kind, key, err)
			}
		}
	}

	if err := r.Patch(ctx, obj, client.Apply,
		client.FieldOwner(FieldManager), client.ForceOwnership); err != nil {
		return fmt.Errorf("apply %s %s: %w", gvk.Kind, key, err)
	}
	return nil
}
```

Notes on choices the code makes:

- **`obj.DeepCopyObject().(client.Object)`** is safe without a checked assertion: every generated Kubernetes type's `DeepCopyObject` returns its own pointer type, which is a `client.Object`. The repo enables no `forcetypeassert` linter.
- **The read goes through `r.Get`**, which is the manager's cache-backed client in production. `SetupWithManager` already watches Services and Deployments (`Owns(...)`), so this read adds no informer and needs no new RBAC; `patch` is already granted. The cache keeps `managedFields` because nothing configures `cache.TransformStripManagedFields`. Adding that transform later would make this migration silently compute a nil patch.
- **A cache that has not seen the object yet** returns NotFound. The migration is skipped for that reconcile, the apply succeeds against the live object, and the migration runs on a later reconcile. `UpgradeManagedFieldsPatch` documents support for running "either before or after initial server-side apply" (`upgrade.go:72`).
- **A stale cache read makes the migration patch fail with a 409 Conflict** (the patch pins the `resourceVersion` it read). `applyOwned` returns that error, so the caller's `failDegraded` sets a transient `Degraded=True` until the next successful reconcile clears it (Task 7). This is accepted, not suppressed: do not add a retry loop or filter `IsConflict` here.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
go build ./... && go vet ./... && go vet -tags=integration ./internal/controller/
go mod tidy && git diff --exit-code go.mod go.sum
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race -v 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | grep -E 'Ran [0-9]+ of|Passed \||^ok|^FAIL'
```

Expected: build and vet print nothing, `git diff --exit-code` exits 0, and the suite reports (measured 2026-10-26):

```
Ran 29 of 29 Specs in ...
SUCCESS! -- 29 Passed | 0 Failed | 0 Pending | 0 Skipped
ok  	github.com/jacaudi/wireguard-operator/internal/controller	...
```

That count includes all four Task 2 specs. A red Task 2 spec here means the wrapping changed behaviour; `MatchError(errProbe)` in the `failDegraded` spec does not go through `applyOwned`, so it is unaffected.

`golangci-lint` is not a gate in this task. If you run it anyway, it reports `failDegraded - result 0 ... is always nil (unparam)` on `apply.go`. That is the staged Task 2 code with no production caller yet, and it disappears in Task 4.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/apply.go internal/controller/apply_test.go
git commit -m "refactor: migrate legacy field ownership before applying

Every Deployment and Service written before this change is owned by the
Update-operation field manager \"manager\" (the production binary's name). A
forced apply transfers only the fields whose value differs, so that manager
keeps co-owning everything the operator renders unchanged -- and the apiserver
never prunes a field another manager still owns. On an existing installation,
disabling the wstunnel sidecar or deleting a service annotation would never
take effect.

applyOwned now folds the legacy manager's ownership into the operator's Apply
entry with csaupgrade.UpgradeManagedFieldsPatch before applying. One extra
write per legacy object, once; objects the operator created by apply are never
patched.

Also wraps applyOwned's errors with the object's kind and key; they still
unwrap to the apiserver's StatusError.

Refs #2"
```

Then confirm the commit is signed (Global Constraints):

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. A missing signature means signing silently failed: stop and report BLOCKED (commit signing unavailable).

---

## Task 4: Deployment path becomes build-then-apply
Kind: refactor

**Files:**
- Modify: `internal/controller/wireguard_controller.go` — the block from the `// deployment` comment through the scheduling-drift check (`:827`–`:921` at `be6d4e4`), rewritten
- Modify: `internal/controller/wireguard_controller.go` — the status block's Deployment read (`:962`–`:975`), and the `reflect` / `slices` imports
- Modify: `internal/controller/wireguard_controller_test.go` — one new spec, one import, and the one-line gofmt fix left by Task 1

**Locate the block by these strings, not by line number:**
- start: the `deploymentFound := &appsv1.Deployment{}` declaration followed by a `Get` for `wireguard.Name + "-dep"`
- end: the closing brace of the block introduced by `// Ensure scheduling settings remain in sync with spec updates.`

**Interfaces:**
- Consumes: `applyOwned` (Task 2, with the migration and wrapped errors from Task 3), `failDegraded` (Task 2); `r.deploymentBuilder.ForWireguard(wg) (*appsv1.Deployment, error)`
- Produces: local variable `desiredDep *appsv1.Deployment`, which the status block reads in place of `deploymentFound`

- [ ] **Step 1: Write the failing test**

`spec.agent.resources` has no drift check today, so changing it currently does nothing. This is the regression test for the actual bug.

Add inside the `Describe("wireguard controller", ...)` closure in `wireguard_controller_test.go`, in the `Context("Wireguard", ...)` block alongside the other specs:

```go
		It("converges a spec field that never had a bespoke drift check", func() {
			created := &v1alpha1.Wireguard{
				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
			}
			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())

			serviceKey := types.NamespacedName{Name: wgName + "-svc", Namespace: wgNamespace}
			// See reconcileServiceWithTypeLoadBalancer: wait for the Service, then call it once.
			Eventually(func() error {
				return k8sClient.Get(context.Background(), serviceKey, &corev1.Service{})
			}, Timeout, Interval).Should(Succeed())
			Expect(reconcileServiceWithTypeLoadBalancer(serviceKey, "localhost")).Should(Succeed())

			deploymentKey := types.NamespacedName{Name: wgName + "-dep", Namespace: wgNamespace}
			Eventually(func() error {
				dep := &appsv1.Deployment{}
				return k8sClient.Get(context.Background(), deploymentKey, dep)
			}, Timeout, Interval).Should(Succeed())

			// Set a field with no drift check in the pre-change controller.
			Eventually(func() error {
				wg := &v1alpha1.Wireguard{}
				if err := k8sClient.Get(context.Background(), wgKey, wg); err != nil {
					return err
				}
				wg.Spec.Agent.Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("123Mi"),
					},
				}
				return k8sClient.Update(context.Background(), wg)
			}, Timeout, Interval).Should(Succeed())

			Eventually(func() string {
				dep := &appsv1.Deployment{}
				if err := k8sClient.Get(context.Background(), deploymentKey, dep); err != nil {
					return "get-failed"
				}
				for _, c := range dep.Spec.Template.Spec.Containers {
					if c.Name == "agent" {
						if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
							return q.String()
						}
						return "unset"
					}
				}
				return "no-agent-container"
			}, Timeout, Interval).Should(Equal("123Mi"))
		})
```

Add to the test file's import block, on the line directly after `corev1 "k8s.io/api/core/v1"` (anywhere else gives Step 5 a second gofmt hunk):

```go
	"k8s.io/apimachinery/pkg/api/resource"
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 2>&1 | tail -30
```

Expected: FAIL with the **final** `Eventually` timing out and reporting `unset`, because no drift check covers `spec.agent.resources`.

Two wrong reds to watch for — neither confirms the premise:

- **`services "vpn-svc" not found` at `wireguard_controller_test.go:65`** means the spec aborted in `reconcileServiceWithTypeLoadBalancer` before reaching the assertion. That helper asserts internally with a bare `Expect`, so it must be preceded by its own `Eventually` on the Service existing, never wrapped in one. Re-check Step 1's code.
- **`spec.ports[...].name`** in the reconcile log means Task 1 was not applied or was incomplete.

**If the spec passes, stop and report:** the premise of the task is wrong.

- [ ] **Step 3: Replace the Deployment block**

Delete everything from the `// deployment` comment line directly above `deploymentFound := &appsv1.Deployment{}` through the closing brace of the scheduling-drift block, and put this in its place. The replacement restates the `// deployment` comment, so keeping the old line would duplicate it.

```go
	// deployment
	//
	// One build, one apply. The apiserver diffs the applied field set against the stored
	// object, so every field the builder renders converges without a bespoke drift check.
	desiredDep, err := r.deploymentBuilder.ForWireguard(wireguard)
	if err != nil {
		return r.failDegraded(ctx, req, err, "DeploymentBuildFailed", "Failed to build deployment")
	}
	if err := r.applyOwned(ctx, desiredDep); err != nil {
		return r.failDegraded(ctx, req, err, "DeploymentApplyFailed", "Failed to apply deployment")
	}
```

- [ ] **Step 4: Point the status block at the apply response**

In the status block, replace all **four** `deploymentFound` references with `desiredDep` (`wireguard_controller.go:964`, `:965`, `:968`, `:975`). After the edit that stanza reads:

```go
		// Deployment status
		depStatus := "Pending"
		depImage := ""
		if desiredDep.Name != "" {
			if desiredDep.Status.ReadyReplicas > 0 {
				depStatus = "Ready"
			}
			for _, c := range desiredDep.Spec.Template.Spec.Containers {
				if c.Name == "agent" {
					depImage = c.Image
					break
				}
			}
		}
		resourcesStatus = append(resourcesStatus, v1alpha1.Resource{Name: desiredDep.Name, Type: "Deployment", Status: depStatus, Image: depImage})
```

The apply response is decoded back into `desiredDep`, so it carries `.Status` from the server. **Do not** add a by-name lookup here — the loop above already resolves the agent by name.

- [ ] **Step 5: Remove now-unused imports**

Deleting the drift checks removes the only uses of `slices` and `reflect` in this file. Verify and remove:

```bash
grep -n "slices\.\|reflect\." internal/controller/wireguard_controller.go
```

If that prints nothing, delete `"reflect"` and `"slices"` from the import block.

Task 1's commit (`e7d4579`) left one gofmt defect: no blank line between the closing `}` of `reconcileServiceWithTypeNodePort` and the comment above `reconcileServiceWithTypeLoadBalancer` (`wireguard_controller_test.go:62-63`). `be6d4e4` was clean. This task already commits that file, so fix it here:

```bash
gofmt -d internal/controller/wireguard_controller_test.go
```

Expected: exactly one hunk, adding that one blank line (measured 2026-10-26). If it shows any other hunk, stop and report it; do not reformat code this task did not write. Otherwise:

```bash
gofmt -w internal/controller/wireguard_controller_test.go
go build ./... && gofmt -l internal/controller/
```

Expected: no output.

- [ ] **Step 6: Run the tests to verify they pass**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race
```

Expected: PASS, including the new spec and the pre-existing `:1236` tolerations and `:1426` userspace specs, which now converge through the apply instead of through their deleted drift checks.

If the tolerations or userspace specs go red on timing rather than on value (the `Eventually` timed out while the value was still moving toward the expected one), raise only that spec's `Eventually` timeout from `Timeout` (10s) to `time.Second * 30` — do not change its assertions. If it is still red at 30s, it is not a timing problem: stop and report.

- [ ] **Step 7: Commit**

```bash
git add internal/controller/wireguard_controller.go internal/controller/wireguard_controller_test.go
git commit -m "fix: reconcile the Deployment declaratively via Server-Side Apply

Replaces the create branch and four hand-written drift checks -- agent image,
userspace flag, wstunnel sidecar, nodeSelector/tolerations -- with a single
build-and-apply. Every field the builder renders now converges, so spec fields
that never had a check (agent.resources, tunnel.port, tunnel.image,
tunnel.resources, enableIpForwardOnPodInit) take effect after first create.

Also fixes three defects that fell out of the old shape: the four checks all
compared against one stale read, so two drifting conditions issued two writes;
the rebuilt Deployment carried no resourceVersion, making every update an
unconditional overwrite; and the status block read a Deployment that could be
several writes out of date.

Refs #2"
```

Then confirm the commit is signed (Global Constraints):

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. A missing signature means signing silently failed: stop and report BLOCKED (commit signing unavailable).

---

## Task 5: Service paths become build-then-apply
Kind: refactor

**Files:**
- Modify: `internal/controller/wireguard_controller.go` — metrics Service block (`:488`–`:516`), the orphaned `svcFound` reset (`:518`), main Service block (`:553`–`:603`), each rewritten. The status block's Service read is deliberately **not** edited: the `svcFound := desiredSvc` alias keeps it correct as-is.
- Modify: `internal/controller/wireguard_controller_test.go` — one new spec

**Locate the blocks by these strings:**
- metrics: the `Get` for `wireguard.Name + "-metrics-svc"`
- main: the `Get` for `wireguard.Name + "-svc"`, through the closing brace of the block commented `// Update service ports if tunnel configuration changed.`

**Interfaces:**
- Consumes: `applyOwned`, `failDegraded`; `r.serviceBuilder.ForWireguard(wg, serviceType)`, `r.serviceBuilder.ForWireguardMetrics(wg)`
- Produces: local `svcFound *corev1.Service` (the main Service's apply response), consumed unchanged by the readiness gates and the status block

- [ ] **Step 1: Write the failing test**

Removing a service annotation has no drift coverage today.

Add inside the same `Context("Wireguard", ...)` block:

```go
		It("removes a service annotation the user has deleted from the spec", func() {
			created := &v1alpha1.Wireguard{
				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
			}
			created.Spec.ServiceAnnotations = map[string]string{
				"probe.test/keep":   "yes",
				"probe.test/remove": "please",
			}
			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())

			serviceKey := types.NamespacedName{Name: wgName + "-svc", Namespace: wgNamespace}
			Eventually(func() bool {
				svc := &corev1.Service{}
				if err := k8sClient.Get(context.Background(), serviceKey, svc); err != nil {
					return false
				}
				_, ok := svc.Annotations["probe.test/remove"]
				return ok
			}, Timeout, Interval).Should(BeTrue(), "annotation should be present after create")

			Eventually(func() error {
				wg := &v1alpha1.Wireguard{}
				if err := k8sClient.Get(context.Background(), wgKey, wg); err != nil {
					return err
				}
				wg.Spec.ServiceAnnotations = map[string]string{"probe.test/keep": "yes"}
				return k8sClient.Update(context.Background(), wg)
			}, Timeout, Interval).Should(Succeed())

			Eventually(func() bool {
				svc := &corev1.Service{}
				if err := k8sClient.Get(context.Background(), serviceKey, svc); err != nil {
					return true
				}
				_, stillThere := svc.Annotations["probe.test/remove"]
				return stillThere
			}, Timeout, Interval).Should(BeFalse(), "removed annotation should be dropped from the Service")

			svc := &corev1.Service{}
			Expect(k8sClient.Get(context.Background(), serviceKey, svc)).Should(Succeed())
			Expect(svc.Annotations).Should(HaveKeyWithValue("probe.test/keep", "yes"))
		})
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 2>&1 | tail -30
```

Expected: FAIL — the removed annotation persists, because the Service is never updated for annotation changes.

- [ ] **Step 3: Replace the metrics Service block**

Replace the whole `svcFound := &corev1.Service{}` / `Get` / create-branch stanza for `-metrics-svc` with:

```go
	// metrics service
	desiredMetricsSvc, err := r.serviceBuilder.ForWireguardMetrics(wireguard)
	if err != nil {
		return r.failDegraded(ctx, req, err, "MetricsServiceBuildFailed", "Failed to build metrics service")
	}
	if err := r.applyOwned(ctx, desiredMetricsSvc); err != nil {
		return r.failDegraded(ctx, req, err, "MetricsServiceApplyFailed", "Failed to apply metrics service")
	}
```

This deletes the `MetricsServicePending` Progressing condition and its early return. That is deliberate: with apply there is no gap between "created" and "usable", so there is nothing to report as pending.

**Also delete the orphaned reset on the next non-blank line.** Between the metrics block and the `serviceType` computation there is a bare re-initialisation:

```go
	svcFound = &corev1.Service{}
```

It reused the variable declared by the metrics block. That declaration is now gone, so this line must go too — it sits *outside* both replaced ranges, and leaving it produces `undefined: svcFound` (Step 4 declares `svcFound` further down the function, which does not help a reference above it). After the deletion, `serviceType := corev1.ServiceTypeLoadBalancer` follows the metrics apply directly.

- [ ] **Step 4: Replace the main Service block**

Replace the `-svc` `Get`, its create branch, and the whole `// Update service ports if tunnel configuration changed.` block with:

```go
	// service
	//
	// Every field the builder renders converges: annotations, target ports, protocols and the
	// second port in dualMode. Server-assigned fields survive because the builder never renders
	// them: clusterIP and ipFamilies are absent from the applied body, and a nodePort of 0 is
	// dropped by omitempty, so the operator never takes ownership of the apiserver's allocation.
	desiredSvc, err := r.serviceBuilder.ForWireguard(wireguard, serviceType)
	if err != nil {
		return r.failDegraded(ctx, req, err, "ServiceBuildFailed", "Failed to build service")
	}
	if err := r.applyOwned(ctx, desiredSvc); err != nil {
		return r.failDegraded(ctx, req, err, "ServiceApplyFailed", "Failed to apply service")
	}
	svcFound := desiredSvc
```

The `svcFound := desiredSvc` alias keeps the readiness gates and the status block unchanged. Leave every reference to `svcFound` below this point exactly as it is.

- [ ] **Step 5: Verify no stale `svcFound` reference remains**

Three things were removed across Steps 3–4: the `svcFound := &corev1.Service{}` declaration in the metrics block, the bare `svcFound = &corev1.Service{}` reset above `serviceType`, and the main Service's own `Get`. The only `svcFound` that should now exist is the `svcFound := desiredSvc` alias from Step 4 and the reads below it.

```bash
go build ./... && go vet ./internal/controller/
grep -n "svcFound" internal/controller/wireguard_controller.go | head -20
```

Expected: build and vet clean; every remaining `svcFound` is a *read* below the apply.

- [ ] **Step 6: Run the tests to verify they pass**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race
```

Expected: PASS, including all three ServiceType end-to-end specs and both tunnel specs.

If a spec fails with `spec.ports[0].name: Required value`, Task 1 was not applied or was reverted — re-check `reconcileServiceWithClusterIP`.

- [ ] **Step 7: Commit**

```bash
git add internal/controller/wireguard_controller.go internal/controller/wireguard_controller_test.go
git commit -m "fix: reconcile both Services declaratively via Server-Side Apply

The main Service's drift check compared only len(Ports) and Ports[0].Port, so
changes to the second port in dualMode, to target ports, to protocols, or to
spec.serviceAnnotations never took effect. The metrics Service was created once
and never reconciled at all.

Server-assigned fields are unaffected: the builder renders neither clusterIP nor
ipFamilies, and a nodePort of 0 is dropped by omitempty, so the operator never
takes ownership of the apiserver's allocation.

Drops the MetricsServicePending and ServicePending progressing conditions --
apply leaves no gap between created and usable for them to describe.

Refs #2"
```

Then confirm the commit is signed (Global Constraints):

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. A missing signature means signing silently failed: stop and report BLOCKED (commit signing unavailable).

---

## Task 6: Counting client and the no-churn spec
Kind: refactor

**Why isolated:** the suite builds exactly one `WireguardReconciler` in `suite_test.go` for all specs, so a global counter would tally every spec's reconciles. This spec counts writes scoped to its own namespace.

**Files:**
- Create: `internal/controller/churn_test.go`
- Modify: `internal/controller/suite_test.go` — a package-level var, and the Wireguard reconciler's construction rewritten to wrap the manager client

**Interfaces:**
- Consumes: Task 3's write behaviour. `applyOwned` sends its legacy field-manager migration as the reconciler's only `types.JSONPatchType` patch; every other write is an apply patch, `Create` or `Update`. `churn_test.go` does not reference `FieldManager`.
- Produces: `countingClient` with `writesTo(ns string, names ...string) int`, `migrationsTo(ns string, names ...string) int` and `reset()`. `reset()` clears `writes` only: migrations are counted from the object's creation. Also a package-level `writeCounter *countingClient`.

**The migration patch counts as a write.** On an object the operator created by apply, there is never a legacy entry, so `UpgradeManagedFieldsPatch` returns nil and `applyOwned` sends nothing (Task 3). The churn objects are all SSA-born, so this spec asserts **zero** migrations across their whole life. That is the only spec proving the nil-patch path issues no request. `depWrites == metricsWrites` would also catch a stray migration, but only one issued after `reset()`.

- [ ] **Step 1: Write the counting client and the no-churn spec**

This spec guards behaviour Tasks 4 and 5 already implemented, so it **passes on first run**; there is no red state to see here. Step 3b below proves it can fail, by planting the defect it exists to catch.

Create `internal/controller/churn_test.go`:

```go
//go:build integration

package controllers

import (
	"context"
	"sync"
	"time"

	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// countingClient records writes the reconciler issues, keyed by namespace/name.
//
// Issue #2's stated acceptance criterion -- watch resourceVersion over several reconciles --
// cannot distinguish a good implementation from a bad one: the apiserver discards a write whose
// defaulted result is byte-identical to what is stored, so an implementation issuing a full PUT
// every reconcile still holds resourceVersion and generation constant.
//
// This counter measures requests ISSUED, which is a different thing from writes PERSISTED. It
// therefore cannot tell Server-Side Apply from a read-modify-write -- both issue one request per
// object per reconcile. What it can tell, precisely, is how many times a SINGLE object is written
// within one reconcile. That is the defect this change removes: four drift checks all comparing
// against one stale read, issuing up to four Deployment writes in a single pass.
type countingClient struct {
	client.Client

	mu     sync.Mutex
	writes map[string]int
	// migrations counts the subset of writes that are applyOwned's legacy field-manager migration
	// (the only JSON patch the reconciler sends). Never cleared by reset: it is counted from birth.
	migrations map[string]int
}

func newCountingClient(c client.Client) *countingClient {
	return &countingClient{Client: c, writes: map[string]int{}, migrations: map[string]int{}}
}

func (c *countingClient) record(obj client.Object, migration bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := obj.GetNamespace() + "/" + obj.GetName()
	c.writes[key]++
	if migration {
		c.migrations[key]++
	}
}

// writesTo returns the total writes issued to the named objects in a namespace.
func (c *countingClient) writesTo(ns string, names ...string) int {
	return c.count(false, ns, names)
}

// migrationsTo returns the legacy field-manager migrations issued to the named objects, ever.
func (c *countingClient) migrationsTo(ns string, names ...string) int {
	return c.count(true, ns, names)
}

func (c *countingClient) count(migrationsOnly bool, ns string, names []string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := c.writes
	if migrationsOnly {
		m = c.migrations
	}
	total := 0
	for _, n := range names {
		total += m[ns+"/"+n]
	}
	return total
}

func (c *countingClient) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = map[string]int{}
}

func (c *countingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.record(obj, false)
	return c.Client.Update(ctx, obj, opts...)
}

func (c *countingClient) Patch(ctx context.Context, obj client.Object, p client.Patch, opts ...client.PatchOption) error {
	c.record(obj, p.Type() == types.JSONPatchType)
	return c.Client.Patch(ctx, obj, p, opts...)
}

func (c *countingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.record(obj, false)
	return c.Client.Create(ctx, obj, opts...)
}

var _ = Describe("no update churn", func() {
	// Timeout and Interval in wireguard_controller_test.go are declared INSIDE its Describe
	// closure, so they are not visible here. Declare our own.
	const (
		churnNS       = "churn-probe"
		churnName     = "churn"
		churnTimeout  = time.Second * 10
		churnInterval = time.Millisecond * 250
	)

	It("applies each owned object at most once per reconcile and persists nothing", func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: churnNS}}
		if err := k8sClient.Create(context.Background(), ns); err != nil {
			Expect(client.IgnoreAlreadyExists(err)).Should(Succeed())
		}

		wg := &v1alpha1.Wireguard{
			ObjectMeta: metav1.ObjectMeta{Name: churnName, Namespace: churnNS},
		}
		wg.Spec.ServiceType = corev1.ServiceTypeClusterIP
		Expect(k8sClient.Create(context.Background(), wg)).Should(Succeed())
		defer func() {
			_ = k8sClient.Delete(context.Background(), wg)
			_ = k8sClient.Delete(context.Background(), &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: churnName + "-dep", Namespace: churnNS}})
		}()

		depKey := types.NamespacedName{Name: churnName + "-dep", Namespace: churnNS}
		Eventually(func() error {
			return k8sClient.Get(context.Background(), depKey, &appsv1.Deployment{})
		}, churnTimeout, churnInterval).Should(Succeed())

		// Record the resourceVersion the Consistently below must hold. This read does not wait
		// for convergence; it only snapshots the Deployment as it is now.
		var settled appsv1.Deployment
		Eventually(func() error {
			return k8sClient.Get(context.Background(), depKey, &settled)
		}, churnTimeout, churnInterval).Should(Succeed())

		writeCounter.reset()

		// Nudge the reconciler by annotating the Wireguard.
		//
		// This is NOT a no-op for every owned object: agent.State embeds the whole Wireguard
		// (wireguard_controller.go, `Server: *wireguard.DeepCopy()`), so the annotation is
		// serialised into state.json and the peer-config Secret is legitimately rewritten. That
		// is why the assertions below name the Deployment and Services explicitly rather than
		// counting everything in the namespace.
		for i := range 3 {
			Eventually(func() error {
				live := &v1alpha1.Wireguard{}
				if err := k8sClient.Get(context.Background(),
					types.NamespacedName{Name: churnName, Namespace: churnNS}, live); err != nil {
					return err
				}
				if live.Annotations == nil {
					live.Annotations = map[string]string{}
				}
				live.Annotations["probe.test/nudge"] = string(rune('a' + i))
				return k8sClient.Update(context.Background(), live)
			}, churnTimeout, churnInterval).Should(Succeed())
		}

		// Nothing the Deployment renders changed, so no write may be persisted.
		Consistently(func() string {
			live := &appsv1.Deployment{}
			if err := k8sClient.Get(context.Background(), depKey, live); err != nil {
				return "get-failed"
			}
			return live.ResourceVersion
		}, "3s", churnInterval).Should(Equal(settled.ResourceVersion),
			"the Deployment must not be rewritten when nothing it renders has changed")

		// The metrics Service is applied exactly once per reconcile: one call site, no branches.
		// It is therefore a reconcile counter. The Deployment must match it one-for-one -- the
		// pre-change code could write the Deployment up to four times in a single reconcile.
		metricsWrites := writeCounter.writesTo(churnNS, churnName+"-metrics-svc")
		depWrites := writeCounter.writesTo(churnNS, churnName+"-dep")
		svcWrites := writeCounter.writesTo(churnNS, churnName+"-svc")

		Expect(metricsWrites).Should(BeNumerically(">", 0), "sanity: the reconciler should have run")
		Expect(depWrites).Should(Equal(metricsWrites),
			"the Deployment must be applied exactly once per reconcile")
		Expect(svcWrites).Should(Equal(metricsWrites),
			"the Service must be applied exactly once per reconcile")

		// SSA-born objects carry no legacy entry, so applyOwned must never send them a migration
		// patch -- counted from creation, not from reset().
		Expect(writeCounter.migrationsTo(churnNS, churnName+"-dep", churnName+"-svc", churnName+"-metrics-svc")).
			Should(BeZero(), "an object the operator created by apply must never be migrated")
	})
})
```

- [ ] **Step 2: Wire the counting client into the suite**

In `internal/controller/suite_test.go`, add a package-level var next to `var k8sClient client.Client`:

```go
var writeCounter *countingClient
```

Then change the reconciler construction from:

```go
	err = (&WireguardReconciler{
		Client:               k8sManager.GetClient(),
```

to:

```go
	writeCounter = newCountingClient(k8sManager.GetClient())

	err = (&WireguardReconciler{
		Client:               writeCounter,
```

Leave `WireguardPeerReconciler` on `k8sManager.GetClient()` — it is out of scope and counting it would add noise.

- [ ] **Step 3: Run the spec**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race
```

Expected: PASS. The `-race` flag is essential here — it is what proves the mutex is doing its job.

Measured 2026-10-26 on the final code state (Tasks 2-7 and 9 applied together in a scratch worktree): PASS under `-race`.

Three failure modes, with different meanings:

- **`Consistently` fails** — the reconciler is persisting a Deployment write on every pass. Capture the two differing `resourceVersion` values and diff the live object against the builder output. **Do not relax the assertion**; a real churn bug is exactly what this spec exists to catch.
- **`depWrites` does not equal `metricsWrites`** — some object is being applied more than once per reconcile. If the Deployment is higher, a second apply call site has crept in. Do not adjust the numbers to match; find the extra call.
- **`migrationsTo` is non-zero** — `applyOwned` sent a migration patch to an object it created itself. Check `legacyFieldManagers` in `apply.go`: it must hold only `"manager"`, never the suite's own `controllers.test`, and the nil-patch guard must be intact.

- [ ] **Step 3b: Prove the spec catches a duplicated apply (temporary mutation, never committed)**

The spec passed on first run, so show it can fail. In `internal/controller/wireguard_controller.go`, duplicate the Deployment apply from Task 4 Step 3, so the block reads:

```go
	if err := r.applyOwned(ctx, desiredDep); err != nil {
		return r.failDegraded(ctx, req, err, "DeploymentApplyFailed", "Failed to apply deployment")
	}
	if err := r.applyOwned(ctx, desiredDep); err != nil {
		return r.failDegraded(ctx, req, err, "DeploymentApplyFailed", "Failed to apply deployment")
	}
```

Run:

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -v -ginkgo.focus='no update churn' 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | grep -E '^\[Fail\]|Ran [0-9]+ of|Passed \||exactly once per reconcile'
```

Expected (measured 2026-10-27):

```
  the Deployment must be applied exactly once per reconcile
[Fail] no update churn [It] applies each owned object at most once per reconcile and persists nothing
Ran 1 of 32 Specs in ...
FAIL! -- 0 Passed | 1 Failed | 0 Pending | 31 Skipped
```

The failure body shows `Expected <int>: 10 to equal <int>: 5` (exact counts may differ; the Deployment count is double the metrics count). The log also shows `apply Deployment churn-probe/churn-dep: metadata.managedFields must be nil` on every reconcile. That is expected: the first apply overwrote `desiredDep` with the server's response (see `applyOwned`'s doc comment), so the second call re-sends managed fields. It is a property of the planted defect, not something to fix.

Now revert: delete the second `if err := r.applyOwned(ctx, desiredDep)` block so exactly one remains.

Run: `grep -c 'r.applyOwned(ctx, desiredDep)' internal/controller/wireguard_controller.go && git diff --exit-code -- internal/controller/wireguard_controller.go`
Expected: `1`, then no diff (the file is back to Task 5's committed state).

Re-run the same focused command. Expected: `SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 31 Skipped`. Do not commit until it is green again; the mutation must not reach Step 4's commit.

- [ ] **Step 4: Commit**

```bash
git add internal/controller/churn_test.go internal/controller/suite_test.go
git commit -m "test: assert each owned object is applied once per reconcile

Counts writes the reconciler issues, keyed by object, alongside a
resourceVersion stability check.

resourceVersion alone cannot distinguish a good implementation from a bad one:
the apiserver discards a write whose defaulted result is byte-identical to what
is stored, so an implementation issuing a full PUT every reconcile still holds
resourceVersion and generation constant.

The counter measures requests issued, not writes persisted, so it cannot tell
Server-Side Apply from a read-modify-write either -- both issue one request per
object per reconcile. What it does measure precisely is how often a single
object is written within one reconcile, which is the defect this change removes:
four drift checks comparing against one stale read could write the Deployment
four times in a single pass.

Also counts legacy field-manager migrations separately, and asserts that an
object the operator created by apply never receives one.

Refs #2"
```

Then confirm the commit is signed (Global Constraints):

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. A missing signature means signing silently failed: stop and report BLOCKED (commit signing unavailable).

---

## Task 7: Convergence specs (removal, external drift, server-assigned fields, adoption, degraded and recovery) and clearing `Degraded`
Kind: refactor

**Files:**
- Modify: `internal/controller/wireguard_controller_test.go`: six new specs and two imports.
- Modify: `internal/controller/wireguard_controller.go`: `updateStatus` (`:123-157` at `be6d4e4`); one block after its `meta.SetStatusCondition` call.

**Interfaces:**
- Consumes: everything from Tasks 2–6. In particular `applyOwned` with the Task 3 migration, `FieldManager`, `resources.NewDeploymentBuilder(scheme *runtime.Scheme, agentImage string, pullPolicy corev1.PullPolicy) *DeploymentBuilder`, and `resources.NewServiceBuilder(scheme *runtime.Scheme) *ServiceBuilder` (`internal/resources`, unchanged by this work).
- Produces: no new symbols. Behaviour change: `updateStatus(ctx, req, cond)` called with `cond.Type == ConditionReady && cond.Status == metav1.ConditionTrue` also sets `Degraded=False` (reason `Reconciled`) in the **same** status write.

**The co-ownership rule (Global Constraints) applied to every spec in this task.** Steps 1 and 3 and Task 5's annotation spec never write an operator-owned object; their removals and nudges go through the `Wireguard` spec only. Step 2 tampers with exactly one field, the agent image, which the operator changes back, so the forced apply takes it over through a conflict. Step 4's two specs are the only ones that `Create` an operator-owned object. One of them then proves a removal, and the other proves the allocations survived. No spec here `Update`s the Deployment or Service in any other way. `reconcileServiceWithTypeLoadBalancer` writes only the Service's `status` subresource, which the operator never applies.

**What this task proves that nothing earlier does:**
- Step 4: B1 is fixed end to end. On a Deployment seeded exactly as the pre-change operator wrote it, disabling the wstunnel sidecar removes it. Step 4b proves it: with the migration neutralised, this spec fails with `["agent", "wstunnel"]`.
- Step 4, Service variant: design §10.2 item 10. The seeded legacy entry owns neither `f:clusterIP` nor `f:nodePort`, and adoption does not move the apiserver's `clusterIP` or `nodePort` allocation.
- Steps 5–7: S1. A `Degraded` left by a failed apply is cleared by the next successful reconcile.

Steps 1–4 cover behaviour Tasks 3–5 already implemented, so they are expected to pass on first run (measured: they do). Steps 5–7 are the red-green cycle for the one production change in this task.

- [ ] **Step 1: Add the sidecar-removal spec**

Inside `Context("Tunnel", ...)`:

```go
		It("removes the wstunnel sidecar when the tunnel is disabled again", func() {
			created := &v1alpha1.Wireguard{
				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
			}
			created.Spec.Tunnel.Enabled = true
			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())

			serviceKey := types.NamespacedName{Name: wgName + "-svc", Namespace: wgNamespace}
			// See reconcileServiceWithTypeLoadBalancer: wait for the Service, then call it once.
			Eventually(func() error {
				return k8sClient.Get(context.Background(), serviceKey, &corev1.Service{})
			}, Timeout, Interval).Should(Succeed())
			Expect(reconcileServiceWithTypeLoadBalancer(serviceKey, "localhost")).Should(Succeed())

			deploymentKey := types.NamespacedName{Name: wgName + "-dep", Namespace: wgNamespace}
			Eventually(func() int {
				dep := &appsv1.Deployment{}
				if err := k8sClient.Get(context.Background(), deploymentKey, dep); err != nil {
					return -1
				}
				return len(dep.Spec.Template.Spec.Containers)
			}, Timeout, Interval).Should(Equal(2), "wstunnel sidecar should be present")

			Eventually(func() error {
				wg := &v1alpha1.Wireguard{}
				if err := k8sClient.Get(context.Background(), wgKey, wg); err != nil {
					return err
				}
				wg.Spec.Tunnel.Enabled = false
				return k8sClient.Update(context.Background(), wg)
			}, Timeout, Interval).Should(Succeed())

			// This is the case equality.Semantic.DeepDerivative cannot detect: a desired list
			// shorter than the actual one is reported as "no change needed".
			Eventually(func() int {
				dep := &appsv1.Deployment{}
				if err := k8sClient.Get(context.Background(), deploymentKey, dep); err != nil {
					return -1
				}
				return len(dep.Spec.Template.Spec.Containers)
			}, Timeout, Interval).Should(Equal(1), "wstunnel sidecar should be removed")
		})
```

- [ ] **Step 2: Add the external-drift spec**

Inside `Context("Wireguard", ...)`:

```go
		It("reverts a Deployment field changed outside the operator", func() {
			created := &v1alpha1.Wireguard{
				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
			}
			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())

			serviceKey := types.NamespacedName{Name: wgName + "-svc", Namespace: wgNamespace}
			// See reconcileServiceWithTypeLoadBalancer: wait for the Service, then call it once.
			Eventually(func() error {
				return k8sClient.Get(context.Background(), serviceKey, &corev1.Service{})
			}, Timeout, Interval).Should(Succeed())
			Expect(reconcileServiceWithTypeLoadBalancer(serviceKey, "localhost")).Should(Succeed())

			deploymentKey := types.NamespacedName{Name: wgName + "-dep", Namespace: wgNamespace}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), deploymentKey, &appsv1.Deployment{})
			}, Timeout, Interval).Should(Succeed())

			Eventually(func() error {
				dep := &appsv1.Deployment{}
				if err := k8sClient.Get(context.Background(), deploymentKey, dep); err != nil {
					return err
				}
				dep.Spec.Template.Spec.Containers[0].Image = "registry.invalid/tampered:latest"
				return k8sClient.Update(context.Background(), dep, client.FieldOwner("kubectl-edit"))
			}, Timeout, Interval).Should(Succeed())

			Eventually(func() string {
				dep := &appsv1.Deployment{}
				if err := k8sClient.Get(context.Background(), deploymentKey, dep); err != nil {
					return "get-failed"
				}
				for _, c := range dep.Spec.Template.Spec.Containers {
					if c.Name == "agent" {
						return c.Image
					}
				}
				return "no-agent"
			}, Timeout, Interval).Should(Equal(wgTestImage))
		})
```

- [ ] **Step 3: Add the server-assigned-field spec**

This guards the largest risk in bringing the Service into scope.

Inside `Context("Wireguard", ...)`, alongside Step 2:

```go
		It("preserves the apiserver's clusterIP and nodePort allocations across reconciles", func() {
			created := &v1alpha1.Wireguard{
				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
			}
			created.Spec.ServiceType = corev1.ServiceTypeNodePort
			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())

			serviceKey := types.NamespacedName{Name: wgName + "-svc", Namespace: wgNamespace}
			Eventually(func() bool {
				svc := &corev1.Service{}
				if err := k8sClient.Get(context.Background(), serviceKey, svc); err != nil {
					return false
				}
				return len(svc.Spec.Ports) > 0 && svc.Spec.Ports[0].NodePort != 0 && svc.Spec.ClusterIP != ""
			}, Timeout, Interval).Should(BeTrue(), "apiserver should have allocated clusterIP and nodePort")

			first := &corev1.Service{}
			Expect(k8sClient.Get(context.Background(), serviceKey, first)).Should(Succeed())

			// Nudge the reconciler; the allocations must not move.
			for i := range 3 {
				Eventually(func() error {
					wg := &v1alpha1.Wireguard{}
					if err := k8sClient.Get(context.Background(), wgKey, wg); err != nil {
						return err
					}
					if wg.Annotations == nil {
						wg.Annotations = map[string]string{}
					}
					wg.Annotations["probe.test/nudge"] = string(rune('a' + i))
					return k8sClient.Update(context.Background(), wg)
				}, Timeout, Interval).Should(Succeed())
			}

			Consistently(func() bool {
				svc := &corev1.Service{}
				if err := k8sClient.Get(context.Background(), serviceKey, svc); err != nil {
					return false
				}
				return svc.Spec.ClusterIP == first.Spec.ClusterIP &&
					len(svc.Spec.Ports) > 0 &&
					svc.Spec.Ports[0].NodePort == first.Spec.Ports[0].NodePort
			}, "3s", Interval).Should(BeTrue(), "server-assigned clusterIP/nodePort must survive apply")
		})
```

- [ ] **Step 4: Add the adoption specs (Deployment, and its Service variant)**

> **Important:** `BeforeEach` deletes Wireguards, peers, Services, Nodes, Secrets and ConfigMaps but **not Deployments**, and envtest runs no garbage collector, so `vpn-dep` leaks between specs. The Deployment spec must delete it and wait for the delete to land before seeding its own. Services need no such step.

Two seeding constraints apply to both specs (design §7.3):
- **Seed before the `Wireguard` exists.** Otherwise the operator creates the object by apply first, and the spec tests nothing.
- **Seed without owner references.** The `Wireguard` has no UID yet; the operator's apply adds the reference. The real builders render exactly what the pre-change operator wrote, and both set an owner reference, so the spec clears it (`legacy.OwnerReferences = nil`). The in-memory `Wireguard` renders identically to the created one: the only CRD default the Deployment builder reads is `tunnel.port: 443`, and the builder falls back to the same `DefaultTunnelPort = 443` (`internal/resources/service.go:33`).

**The adoption wait keys on `ownerReferences`, deliberately.** After the migration patch alone, `managedFields` already shows `FieldManager` and no `manager`. Waiting on that could pass in the gap between the migration and the apply. Only the operator's apply adds an owner reference.

Inside `Context("Wireguard", ...)`, alongside Steps 2 and 3:

```go
		It("adopts a Deployment written before Server-Side Apply, so a later removal converges", func() {
			deploymentKey := types.NamespacedName{Name: wgName + "-dep", Namespace: wgNamespace}

			// The suite leaks Deployments between specs; start from a clean slate.
			stale := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deploymentKey.Name, Namespace: deploymentKey.Namespace},
			}
			_ = k8sClient.Delete(context.Background(), stale)
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), deploymentKey, &appsv1.Deployment{})
				return err != nil
			}, Timeout, Interval).Should(BeTrue(), "leftover Deployment should be gone")

			// Seed the legacy object BEFORE the Wireguard exists -- otherwise the operator creates
			// the Deployment by apply first and this spec tests nothing. The real builder renders
			// exactly what the pre-change operator wrote, wstunnel sidecar included; its owner
			// reference is dropped because the Wireguard has no UID yet (the operator's apply adds
			// one). "manager" is the field manager the production binary recorded before this change.
			// This is the only Create of an operator-owned object in the suite (design §7.3).
			owner := &v1alpha1.Wireguard{
				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
			}
			owner.Spec.Tunnel.Enabled = true
			legacy, err := resources.NewDeploymentBuilder(k8sClient.Scheme(), wgTestImage, corev1.PullIfNotPresent).
				ForWireguard(owner)
			Expect(err).ShouldNot(HaveOccurred())
			legacy.OwnerReferences = nil
			Expect(k8sClient.Create(context.Background(), legacy, client.FieldOwner("manager"))).Should(Succeed())
			seededGeneration := legacy.Generation

			Expect(k8sClient.Create(context.Background(), owner)).Should(Succeed())

			serviceKey := types.NamespacedName{Name: wgName + "-svc", Namespace: wgNamespace}
			// See reconcileServiceWithTypeLoadBalancer: wait for the Service, then call it once.
			Eventually(func() error {
				return k8sClient.Get(context.Background(), serviceKey, &corev1.Service{})
			}, Timeout, Interval).Should(Succeed())
			Expect(reconcileServiceWithTypeLoadBalancer(serviceKey, "localhost")).Should(Succeed())

			// Adopted: the owner reference proves the operator's apply ran (the migration patch
			// alone adds none).
			Eventually(func() bool {
				dep := &appsv1.Deployment{}
				if err := k8sClient.Get(context.Background(), deploymentKey, dep); err != nil {
					return false
				}
				var managers []string
				for _, mf := range dep.ManagedFields {
					managers = append(managers, mf.Manager)
				}
				return len(dep.OwnerReferences) > 0 && slices.Contains(managers, FieldManager)
			}, Timeout, Interval).Should(BeTrue(), "the operator should have applied over the legacy Deployment")

			adopted := &appsv1.Deployment{}
			Expect(k8sClient.Get(context.Background(), deploymentKey, adopted)).Should(Succeed())
			Expect(adopted.Generation).Should(Equal(seededGeneration),
				"adopting identical content must not change the stored spec or trigger a rollout")

			Eventually(func() error {
				wg := &v1alpha1.Wireguard{}
				if err := k8sClient.Get(context.Background(), wgKey, wg); err != nil {
					return err
				}
				wg.Spec.Tunnel.Enabled = false
				return k8sClient.Update(context.Background(), wg)
			}, Timeout, Interval).Should(Succeed())

			// The proof that adoption is complete. Without the legacy-manager migration in
			// applyOwned, "manager" still co-owns the sidecar and the apiserver never prunes it.
			Eventually(func() []string {
				dep := &appsv1.Deployment{}
				if err := k8sClient.Get(context.Background(), deploymentKey, dep); err != nil {
					return nil
				}
				var names []string
				for _, c := range dep.Spec.Template.Spec.Containers {
					names = append(names, c.Name)
				}
				return names
			}, Timeout, Interval).Should(Equal([]string{"agent"}), "the legacy-owned wstunnel sidecar must be removed")

			removed := &appsv1.Deployment{}
			Expect(k8sClient.Get(context.Background(), deploymentKey, removed)).Should(Succeed())
			var managers []string
			for _, mf := range removed.ManagedFields {
				managers = append(managers, mf.Manager)
			}
			Expect(managers).ShouldNot(ContainElement("manager"), "the legacy Update entry must be gone")
		})

		It("adopts a legacy Service without moving its clusterIP or nodePort allocation", func() {
			// Spec 6's Service variant (design §10.2 item 10): if the legacy entry owned an
			// allocated field, migration would hand it to the operator, whose next apply
			// relinquishes it. The assertions below pin that it owns neither clusterIP nor
			// nodePort, and that both allocations survive adoption.
			// Same seeding rules as above; BeforeEach already deletes Services.
			serviceKey := types.NamespacedName{Name: wgName + "-svc", Namespace: wgNamespace}
			owner := &v1alpha1.Wireguard{
				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
			}
			owner.Spec.ServiceType = corev1.ServiceTypeNodePort
			legacy, err := resources.NewServiceBuilder(k8sClient.Scheme()).ForWireguard(owner, corev1.ServiceTypeNodePort)
			Expect(err).ShouldNot(HaveOccurred())
			legacy.OwnerReferences = nil
			Expect(k8sClient.Create(context.Background(), legacy, client.FieldOwner("manager"))).Should(Succeed())

			seeded := &corev1.Service{}
			Expect(k8sClient.Get(context.Background(), serviceKey, seeded)).Should(Succeed())
			Expect(seeded.Spec.ClusterIP).ShouldNot(BeEmpty())
			Expect(seeded.Spec.Ports).ShouldNot(BeEmpty())
			Expect(seeded.Spec.Ports[0].NodePort).ShouldNot(BeZero())

			// The apiserver allocated clusterIP and nodePort itself, so the legacy Update entry
			// must not own them: there is nothing allocated for the migration to hand over.
			var legacyFields string
			for _, mf := range seeded.ManagedFields {
				if mf.Manager == "manager" && mf.Operation == metav1.ManagedFieldsOperationUpdate && mf.FieldsV1 != nil {
					legacyFields = string(mf.FieldsV1.Raw)
				}
			}
			Expect(legacyFields).ShouldNot(BeEmpty(), "the seed must carry an Update entry for manager")
			Expect(legacyFields).ShouldNot(ContainSubstring(`"f:clusterIP"`), "the legacy entry must not own clusterIP")
			Expect(legacyFields).ShouldNot(ContainSubstring(`"f:nodePort"`), "the legacy entry must not own nodePort")

			Expect(k8sClient.Create(context.Background(), owner)).Should(Succeed())

			adopted := &corev1.Service{}
			Eventually(func() bool {
				if err := k8sClient.Get(context.Background(), serviceKey, adopted); err != nil {
					return false
				}
				var managers []string
				for _, mf := range adopted.ManagedFields {
					managers = append(managers, mf.Manager)
				}
				return len(adopted.OwnerReferences) > 0 &&
					slices.Contains(managers, FieldManager) && !slices.Contains(managers, "manager")
			}, Timeout, Interval).Should(BeTrue(), "the operator should adopt the legacy Service")

			Expect(adopted.Spec.ClusterIP).Should(Equal(seeded.Spec.ClusterIP))
			Expect(adopted.Spec.Ports).ShouldNot(BeEmpty())
			Expect(adopted.Spec.Ports[0].NodePort).Should(Equal(seeded.Spec.Ports[0].NodePort))
		})
```

Assertion order in the Deployment spec is deliberate. With the migration neutralised, the generation check still passes and the removal `Eventually` is what fails. That makes the removal the proof of B1, which is what the design-gate ruling requires. `manager`'s absence is asserted last, as a supporting fact.

The generation assertions in both adoption specs cover design §10.2 item 11, which reasoned without measuring that re-defaulting after migration stores an identical spec. Measured 2026-10-26: `metadata.generation` is unchanged across adoption, so adoption triggers no rollout.

The file's complete import block after this task. `slices` and `internal/resources` are new here; `resource` arrived in Task 4:

```go
import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
	"github.com/jacaudi/wireguard-operator/internal/resources"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)
```

- [ ] **Step 4b: Prove the adoption specs depend on the migration (temporary mutation, never committed)**

Steps 1–4 pass on first run, so show the three migration-dependent specs can fail. In `internal/controller/apply.go`, neutralise the migration by changing

```go
var legacyFieldManagers = sets.New("manager")
```

to

```go
var legacyFieldManagers = sets.New("not-a-manager")
```

Run:

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -v -ginkgo.focus='adopts a|migrates a legacy' 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | grep -E '^\[Fail\]|Ran [0-9]+ of|Passed \||\["agent", "wstunnel"\]'
```

Expected (measured 2026-10-27):

```
        <[]string | len:2, cap:2>: ["agent", "wstunnel"]
[Fail] applyOwned [It] migrates a legacy Update-owned object so a field it stops rendering is removed
[Fail] wireguard controller Wireguard [It] adopts a Deployment written before Server-Side Apply, so a later removal converges
[Fail] wireguard controller Wireguard [It] adopts a legacy Service without moving its clusterIP or nodePort allocation
Ran 3 of 37 Specs in ...
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 34 Skipped
```

The `[Fail]` lines can appear in any order (Ginkgo v1 shuffles top-level containers on each seed); match the set, not the order.


`["agent", "wstunnel"]` is spec 6 failing at the removal: without the migration, `manager` still co-owns the sidecar and the apiserver never prunes it. Any spec passing here means it does not depend on the migration: stop and report.

Now revert `legacyFieldManagers` to `sets.New("manager")`.

Run: `git diff --exit-code -- internal/controller/apply.go`
Expected: no output (the file is back to Task 3's committed state).

Re-run the same focused command. Expected: `SUCCESS! -- 3 Passed | 0 Failed | 0 Pending | 34 Skipped`. Do not continue until it is green again.

- [ ] **Step 5: Add the permanent-failure-and-recovery spec**

Also inside `Context("Wireguard", ...)`:

```go
		It("reports Degraded while an apply can never succeed, and clears it once one does", func() {
			// nodePort on a ClusterIP Service is rejected by the apiserver, permanently.
			created := &v1alpha1.Wireguard{
				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
			}
			created.Spec.ServiceType = corev1.ServiceTypeClusterIP
			created.Spec.NodePort = 30123
			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())

			Eventually(func() string {
				wg := &v1alpha1.Wireguard{}
				if err := k8sClient.Get(context.Background(), wgKey, wg); err != nil {
					return "get-failed"
				}
				for _, c := range wg.Status.Conditions {
					if c.Type == ConditionDegraded && c.Status == metav1.ConditionTrue {
						return c.Reason
					}
				}
				return "no-degraded-condition"
			}, Timeout, Interval).Should(Equal("ServiceApplyFailed"))

			// Remove the cause. The next reconcile applies every object, so Ready must be True and
			// Degraded must be False -- not left True beside it from the earlier failure.
			Eventually(func() error {
				wg := &v1alpha1.Wireguard{}
				if err := k8sClient.Get(context.Background(), wgKey, wg); err != nil {
					return err
				}
				wg.Spec.NodePort = 0
				return k8sClient.Update(context.Background(), wg)
			}, Timeout, Interval).Should(Succeed())

			Eventually(func() map[string]metav1.ConditionStatus {
				wg := &v1alpha1.Wireguard{}
				if err := k8sClient.Get(context.Background(), wgKey, wg); err != nil {
					return nil
				}
				got := map[string]metav1.ConditionStatus{}
				for _, c := range wg.Status.Conditions {
					got[c.Type] = c.Status
				}
				return got
			}, Timeout, Interval).Should(SatisfyAll(
				HaveKeyWithValue(ConditionReady, metav1.ConditionTrue),
				HaveKeyWithValue(ConditionDegraded, metav1.ConditionFalse),
			), "a successful reconcile must clear Degraded")
		})
```

> If the apiserver in envtest accepts `nodePort` on a ClusterIP Service rather than rejecting it, the first half of this spec cannot be written this way. In that case **stop and report** rather than weakening the assertion — the design's §3.4 premise would need revisiting. (Measured 2026-10-26: the apiserver rejects it, and the spec reaches `ServiceApplyFailed`.)

- [ ] **Step 6: Run the full suite to verify exactly the recovery assertion fails**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race -v 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | grep -E '^\[Fail\]|Ran [0-9]+ of|Passed \||"Degraded": "True"'
```

Expected (measured 2026-10-26):

```
            "Degraded": "True",
[Fail] wireguard controller Wireguard [It] reports Degraded while an apply can never succeed, and clears it once one does
Ran 38 of 38 Specs in ...
FAIL! -- 37 Passed | 1 Failed | 0 Pending | 0 Skipped
```

The failure message reads `a successful reconcile must clear Degraded` and shows the conditions map with `"Ready": "True"` beside `"Degraded": "True"`. That is S1: `updateStatus` upserts by type, and the success path writes only `Ready`. Any other red spec is a real defect. Fix it before Step 7; do not weaken its assertion.

- [ ] **Step 7: Clear `Degraded` in the same status write as `Ready=True`**

In `internal/controller/wireguard_controller.go`, inside `updateStatus`, replace the single line

```go
	meta.SetStatusCondition(&latest.Status.Conditions, cond)
```

with:

```go
	meta.SetStatusCondition(&latest.Status.Conditions, cond)

	// Ready=True is written only once every apply in the reconcile has succeeded, so it also clears
	// a Degraded left by an earlier failure. It must ride in this same write: a separate
	// updateStatus call for Degraded would set the summary below to Error, and a second write that
	// hits a Conflict is swallowed, so the clearing could be lost without trace.
	if cond.Type == ConditionReady && cond.Status == metav1.ConditionTrue {
		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               ConditionDegraded,
			Status:             metav1.ConditionFalse,
			Reason:             "Reconciled",
			Message:            "All owned resources applied",
			ObservedGeneration: cond.ObservedGeneration,
		})
	}
```

Leave the summary `switch` below it, and the `Ready=True` call site at the end of `Reconcile` (`Reason: "Active"`), exactly as they are.

**Why inside `updateStatus` and not at the call site.** The binding rule (design §3.4, *Clearing `Degraded`*) is that both conditions land in **one** status write. A second `updateStatus` call for `Degraded=False` would fail that rule twice. The switch maps `ConditionDegraded` to `Status.Status = Error` whatever the condition's `Status` says (`:144-145`). And `updateStatus` swallows a `Conflict` (`:152-154`), so the clearing write could be lost without trace. Setting both conditions on `latest` before its single `Status().Update` meets the rule with no signature change. `Ready=True` is written only at the end of `Reconcile`, after all three applies have returned without error, so "Ready=True" and "every apply succeeded" are the same event. `meta.SetStatusCondition` moves `LastTransitionTime` only when the status actually changes, so writing `Degraded=False` on every successful reconcile persists nothing new after the first.

**Visible consequence:** every `Wireguard` that reaches Ready now carries a `Degraded=False` condition, where before it carried none. No existing spec asserts the condition count. This follows the Kubernetes API convention that a known condition is reported `False` rather than omitted.

**Residual, accepted (design §3.4):** a reconcile whose applies succeed but which stops at a readiness gate leaves `Degraded=True` until a later reconcile reaches Ready. **Pre-existing, out of scope:** `Progressing=True` is never cleared either; do not fix it here.

- [ ] **Step 8: Run the full suite to verify everything passes**

```bash
go build ./... && go vet ./... && go vet -tags=integration ./internal/controller/
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race -v 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | grep -E 'Ran [0-9]+ of|Passed \||^ok|^FAIL'
```

Expected: build and vet print nothing, and (measured 2026-10-26):

```
Ran 38 of 38 Specs in ...
SUCCESS! -- 38 Passed | 0 Failed | 0 Pending | 0 Skipped
ok  	github.com/jacaudi/wireguard-operator/internal/controller	...
```

38 = 27 at Task 2 + 2 (Task 3) + 1 (Task 4) + 1 (Task 5) + 1 (Task 6) + 6 (this task).

- [ ] **Step 9: Commit**

```bash
git add internal/controller/wireguard_controller.go internal/controller/wireguard_controller_test.go
git commit -m "fix: clear Degraded on success; cover removal, drift, adoption, failure

updateStatus now sets Degraded=False in the same status write as Ready=True.
Before, one transient apply failure left Degraded=True beside Ready=True
indefinitely: conditions upsert by type and the success path wrote only Ready.
A separate write would have set the summary to Error and could be lost to a
swallowed Conflict.

Six specs the old design's suite could not have: sidecar removal (the case
DeepDerivative cannot detect); reversion of a field changed by another field
manager; survival of the apiserver's clusterIP and nodePort allocations;
adoption of a Deployment written before Server-Side Apply, proven by removing
its legacy-owned sidecar; adoption of a legacy Service without moving its
allocations; and a Degraded condition on an apply that can never succeed,
cleared once the cause is removed.

Refs #2"
```

Then confirm the commit is signed (Global Constraints):

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. A missing signature means signing silently failed: stop and report BLOCKED (commit signing unavailable).

---

## Task 8: Bump vulnerable dependencies
Kind: refactor

**Depends on:** Task 7 (committed). Task 9 depends on this task.

**Files:**
- Modify: `go.mod`, `go.sum`

**Why:** `govulncheck` reports reachable vulnerabilities at base `be6d4e4`, and the owner ruled to fix them in this branch (Run contract, *Known vulnerabilities*). This task changes no Go source. It is the only task that edits `go.mod` or `go.sum`, so Task 9 Step 2b can still expect a clean `go mod tidy -diff`.

**What the bump clears (measured 2026-10-28 on `e7d4579`; the bumped `go.mod` is byte-identical with and without Tasks 3-7 applied, and the final code passes every step below):**

| Move | From | To | Clears |
|---|---|---|---|
| `golang.org/x/net` | v0.55.0 | v0.60.0 | GO-2026-6617, -6612, -6611, -6610, -6603 |
| `google.golang.org/grpc` | v1.82.1 | v1.83.1 | GO-2026-6348 |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` (and `otlptrace`, `otel`, `otel/sdk`, `otel/metric`, `otel/trace`) | v1.19.0 / v1.43.0 | v1.45.0 | GO-2026-6505 |
| `toolchain` | go1.26.7 | go1.26.10 | GO-2026-6809 (`crypto/x509`, fixed in go1.26.10 and go1.27.3) and, when CI runs go1.26.7 as `go.mod` pins, six more standard-library IDs fixed in go1.26.9 (GO-2026-6599, -6600, -6605, -6607, -6608, -6613) |

Knock-on moves that `go get` makes by itself: `go 1.26` becomes `go 1.26.0`; `x/crypto` v0.57.0, `x/sys` v0.48.0, `x/sync` v0.23.0, `x/term` v0.46.0, `x/text` v0.42.0; `otelhttp` v0.61.0; `go.opentelemetry.io/proto/otlp` v1.11.0; `grpc-gateway/v2` v2.29.0; `cenkalti/backoff` v4.2.1 (removed) to `backoff/v5` v5.0.3; `logr` v1.4.4; `httpsnoop` v1.0.4; `genproto/googleapis/{api,rpc}` 20260803160001-6ac0973c030d.

**Not moved, by the owner's binding ruling:** `k8s.io/*` and `sigs.k8s.io/controller-runtime`. The command below does not touch them, and Step 2 checks that.

**Which Go runs the scan.** CI installs Go from `go.mod` (`go-version-file: go.mod`, `.github/workflows/ci-integration.yml:34` and the setup action at `.github/actions/setup/action.yml:70`), so after this task CI runs **go1.26.10**. This machine's `go` is **go1.27.2**, which is newer than the `toolchain` line and therefore wins. Step 6 scans under both.

- [ ] **Step 1: Confirm the tree is committed and clean**

Run: `git status --short --untracked-files=no`
Expected: no output. Anything listed means an earlier task is uncommitted: stop and report it.

- [ ] **Step 2: Apply the bump**

```bash
go get golang.org/x/net@v0.60.0 google.golang.org/grpc@v1.83.1 go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@v1.45.0 toolchain@go1.26.10 && go mod tidy
```

Expected: exit 0, with these lines among the `go: upgraded` output: `go 1.26 => 1.26.0`, `toolchain go1.26.7 => go1.26.10`, `golang.org/x/net v0.55.0 => v0.60.0`, `google.golang.org/grpc v1.82.1 => v1.83.1`, `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.19.0 => v1.45.0`, and `go: added github.com/cenkalti/backoff/v5 v5.0.3`. `otlptracegrpc` is named because upgrading it pulls `otlptrace`, `backoff/v5` and the newer `genproto`; naming `otlptrace` alone leaves those behind (measured: the resulting `go.mod` differed from the target).

Run: `git diff go.mod | grep -E '^[+-]' | grep -E 'k8s.io|controller-runtime'`
Expected: no output. Any line printed means a `k8s.io/*` or controller-runtime module moved, which the owner ruled out: run `git checkout -- go.mod go.sum` and stop with `PLAN: BLOCKED — dependency bump moved a pinned module`.

- [ ] **Step 3: Read the `go.mod` diff**

Run: `git diff go.mod`
Expected: `go 1.26` becomes `go 1.26.0`; `toolchain go1.26.7` becomes `toolchain go1.26.10`; and among the changed `require` lines `golang.org/x/net v0.55.0` becomes `v0.60.0`, `google.golang.org/grpc v1.82.1` becomes `v1.83.1`, `go.opentelemetry.io/otel` and the `otlptrace`, `otlptracegrpc`, `sdk`, `metric` and `trace` modules become `v1.45.0`, and `otelhttp` becomes `v0.61.0`. No `k8s.io/*` line changes.

Run: `git diff --stat`
Expected: `go.mod | 46 ++++++++++++++++----------------` and `go.sum | 94 ++++++++++++++++++++++++++++++++----------------------------------`, then `2 files changed, 69 insertions(+), 71 deletions(-)`. Small differences in those counts are tolerable only if the Step 2 and Step 3 line checks above hold; a different set of changed modules is not, so stop and report it.

- [ ] **Step 4: Build and vet**

Run: `go build ./... && go vet ./... && go vet -tags=integration ./internal/controller/`
Expected: exit 0, no output.

- [ ] **Step 5: Unit tests, then the integration suite, both under `-race`**

Run: `rtk proxy go test ./... -race` (the RTK hook otherwise condenses the output to one summary line)
Expected: exit 0. `internal/agent`, `internal/ipam` and `internal/iptables` report `ok`; `api/v1alpha1`, `cmd/agent`, `cmd/manager`, `internal/resources` and `internal/wireguard` report `[no test files]`; `internal/controller` also reports `[no test files]` because its test files carry the `integration` tag.

Run: `KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race`
Expected: `ok  	github.com/jacaudi/wireguard-operator/internal/controller` followed by a duration (78.7s and 89.4s measured on this machine, with the base code and with the Tasks 3-7 code). Any `FAIL` is a real failure introduced by the bump: report it, and do not edit `go.mod` by hand or revert Task 7.

- [ ] **Step 6: Vulnerability scan, under this machine's Go and under the Go that CI uses**

Run: `go version && task go:vuln 2>&1 | grep -oE 'GO-[0-9]{4}-[0-9]+' | sort -u`, then `task go:vuln 2>&1 | tail -1`
Expected on go1.27.2 (measured 2026-10-28, `govulncheck` v1.8.0, database updated 2026-10-27): exactly one ID, `GO-2026-6809`, and the last line `task: Failed to run task "go:vuln": exit status 3` (`task`'s own exit status is `201`). The finding reads `Found in: crypto/x509@go1.27.2`, `Fixed in: crypto/x509@go1.27.3`, with the example trace `agent.StartMetricsServer calls http.Server.ListenAndServe, which eventually calls x509.Certificate.Verify`. This is an **environment residual**: it lives in this machine's standard library, not in `go.mod`, and no change to `go.mod` clears it on go1.27.2. If `go version` prints go1.27.3 or later, expect no ID at all and exit 0. Any ID other than `GO-2026-6809` means the bump is incomplete: stop and report it.

Run: `GOTOOLCHAIN=go1.26.10 task go:vuln 2>&1 | tail -8`, then `GOTOOLCHAIN=go1.26.10 govulncheck ./... > /dev/null; echo "exit=$?"`
Expected: `No vulnerabilities found.` and `Your code is affected by 0 vulnerabilities.`, then `exit=0`. This is the toolchain CI runs. The first use downloads go1.26.10 (`go: downloading go1.26.10`); that is expected. The footer also says `This scan also found 1 vulnerability in packages you import and 3 vulnerabilities in modules you require, but your code doesn't appear to call these vulnerabilities.` Those four are not reachable, so they do not gate: `GO-2026-6443` (`grpc` v1.83.1, fixed in v1.83.2), `GO-2026-6866` and `GO-2026-6860` (`x/net` v0.60.0, fixed in v0.62.0), and `GO-2026-5932` (`x/crypto` v0.57.0, no fixed version listed). They are follow-ups, deliberately not bumped here.

For reference, the base scan before this task: `e7d4579` reports 8 reachable IDs under go1.27.2 (GO-2026-6809, -6617, -6612, -6611, -6610, -6603, -6505, -6348) and 14 under go1.26.7, the toolchain CI would have used.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum
git commit -m "fix(deps): bump x/net, grpc, otel and the Go toolchain to clear reachable vulnerabilities

govulncheck reported reachable vulnerabilities at the base commit. Bumped:
golang.org/x/net v0.55.0 -> v0.60.0 (GO-2026-6617, -6612, -6611, -6610, -6603),
google.golang.org/grpc v1.82.1 -> v1.83.1 (GO-2026-6348), the OpenTelemetry
modules to v1.45.0 with otelhttp v0.61.0 (GO-2026-6505), and the toolchain
go1.26.7 -> go1.26.10 (GO-2026-6809 and six further standard-library IDs).

Under go1.26.10, the Go that CI installs from go.mod, govulncheck reports no
vulnerabilities. k8s.io/* and controller-runtime are deliberately unchanged.

Refs #2"
```

Then confirm the commit is signed (Global Constraints):

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. A missing signature means signing silently failed: stop and report BLOCKED (commit signing unavailable).

---

## Task 9: Documentation and final verification
Kind: refactor

**Depends on:** Tasks 1-8.

**Files:**
- Modify: `docs/ROADMAP.md` — § Phase 0.2: the three-line paragraph (`:170-172`) rewritten, and the checkboxes (`:174-176`)
- Modify: `.golangci.yml` — the ratchet comment, `:89-92` (M5), rewritten

- [ ] **Step 1: Correct the stale line numbers in the roadmap**

`docs/ROADMAP.md` § Phase 0.2 cites `:824` / `:844` / `:861` / `:871`, which predate PRs #39 and #49. Find them:

Run: `grep -n "824\|844\|861\|871" docs/ROADMAP.md`
Expected: exactly one line:

```
171:(`:824`), userspace flag (`:844`), tunnel sidecar (`:861`), scheduling settings (`:871`).
```

If it prints anything else, stop and report: the roadmap has changed since this plan was written.

Replace exactly these three lines (`docs/ROADMAP.md:170-172`), the paragraph directly under the `### 0.2 Replace trigger-based Deployment drift checks …` heading:

```markdown
The Deployment is only rebuilt when one of four specific checks fires: agent image
(`:824`), userspace flag (`:844`), tunnel sidecar (`:861`), scheduling settings (`:871`).
Any new spec field would need its own bespoke check or changes would silently not apply.
```

with this description, which cannot go stale:

```markdown
The Deployment was rebuilt only when one of four hand-written checks fired — agent image drift,
userspace flag toggle, wstunnel sidecar toggle, and nodeSelector/tolerations change — plus a
create branch. All five rebuilt the same desired Deployment. Replaced with a single Server-Side
Apply through `applyOwned`; see
`docs/designs/2026-08-29-declarative-deployment-reconciliation-design.md`.
```

The three acceptance checkboxes immediately below that paragraph (`docs/ROADMAP.md:174-176`) are still unticked. **Tick the first and third; leave the second unticked** and append to it ` — except spec.metric, tracked as #50`. Leaving all three unticked under a past-tense paragraph would make the roadmap contradict itself.

- [ ] **Step 1b: Correct the stale lint-ratchet comment (M5)**

`.golangci.yml:89-92` says the reconcile loop's gocyclo hits are "`Reconcile` at cyclomatic complexity 121, `updateWireguardPeers` at 42), which issue #2 replaces wholesale". Both halves are now wrong. Those were the figures at lint adoption, not at this work's base (the design gate confirmed 132 and 43 at `be6d4e4`). And this work replaces only `Reconcile`'s Deployment and Service drift checks; `updateWireguardPeers` is deliberately untouched (Global Constraints, *Do not touch*). Also, only two of the five gocyclo hits are the reconcile loop. The other three are `WireguardPeerReconciler.Reconcile`, `(*Wireguard).Sync` and `createPeersConfiguration` (measured 2026-10-26).

Replace exactly these four lines:

```yaml
  # 2 modernize, 1 prealloc. The five gocyclo hits are the reconcile loop
  # (`Reconcile` at cyclomatic complexity 121, `updateWireguardPeers` at 42),
  # which issue #2 replaces wholesale — absorbing them by hand would mean
  # editing code that is about to be rewritten.
```

with:

```yaml
  # 2 modernize, 1 prealloc. Two of the five gocyclo hits are the reconcile
  # loop: `Reconcile` (121 at adoption, 132 by be6d4e4) and
  # `updateWireguardPeers` (42, then 43). Issue #2 replaced only Reconcile's
  # Deployment and Service drift checks with Server-Side Apply, taking it to
  # 97, and deliberately left updateWireguardPeers alone. Decomposing either is
  # separate work, so both stay ratcheted rather than absorbed by hand.
```

Verify that only the comment changed, and that the file still parses:

```bash
git diff --stat .golangci.yml
golangci-lint config verify
```

Expected: `1 file changed, 6 insertions(+), 4 deletions(-)`, and `config verify` prints nothing and exits 0. If your `golangci-lint` has no `config verify` subcommand, Step 2's `task ci` is the parse check.

- [ ] **Step 2: Run the full local gate**

```bash
task ci
```

`task ci` runs `repo:ci`, then `go:ci`, whose stages are `fmt:check`, `tidy:check`, `lint`, `vuln`, `test` in that order, and it **stops at the first failing stage**. On this machine it is expected to stop at `go:lint` (measured 2026-10-27 on the final Tasks 2–7 code):

```
task: Failed to run task "ci": task: Failed to run task "go:ci": task: Failed to run task "go:lint": exit status 1
```

preceded by `2 issues:` / `* typecheck: 2`, both of the form `could not load export data: ... export data version 5 is greater than maximum supported version 4`. The `repo:*` stages, `go:fmt:check` and `go:tidy:check` will have passed before it. If `task ci` stops at any other stage, or lint reports anything other than `typecheck`, that is a real failure: fix it on **changed** lines only. Because `go:vuln` and `go:test` never run from `task ci` here, Steps 2a–2d below run the lint and each later stage explicitly.

- [ ] **Step 2a: Lint under the matching toolchain**

The golangci-lint ratchet (`new-from-merge-base: origin/main`) reports on the lines this work touched. Fix anything it reports on **changed** lines. Gate on the printed `0 issues.` line, never on the exit status (Global Constraints).

If golangci-lint emits `Can't process results by diff processor: can't prepare diff by revgrep`, the merge base is missing locally and it is reporting the whole tree (~113 pre-existing findings). Run `git fetch origin main` and retry — do not "fix" the pre-existing findings.

If golangci-lint reports only `typecheck` errors of the form `could not load export data: ... export data version 5 is greater than maximum supported version 4`, the code is fine. The local `golangci-lint` binary was built with an older Go than the `go` on `PATH` (observed 2026-10-26: binary built with go1.27.1, `go` 1.27.2). Rerun the lint under the toolchain the binary was built with:

```bash
golangci-lint version   # note "built with goX.Y.Z"
TC=$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-goX.Y.Z.$(go env GOOS)-$(go env GOARCH)
PATH=$TC/bin:$PATH GOTOOLCHAIN=local GOROOT=$TC golangci-lint run ./...
```

Expected: `0 issues.` (measured 2026-10-27, and again 2026-10-28 with the Task 8 bump, with the full Tasks 2–7 code in a scratch worktree under `/private/tmp`, `X.Y.Z` = `1.27.1`). If `$TC` does not exist, run `GOTOOLCHAIN=goX.Y.Z go version` once to download it.

- [ ] **Step 2b: Module tidiness**

Run: `task go:tidy:check`
Expected: exit 0, and the only output is `task: [go:tidy:check] go mod tidy -diff` (no diff follows it). Task 8 bumped versions and added `backoff/v5` as an indirect module, and left `go.mod` tidy; this task adds nothing. Any diff is a defect to report, not to commit.

- [ ] **Step 2c: Vulnerability scan**

Run: `task go:vuln 2>&1 | grep -oE 'GO-[0-9]{4}-[0-9]+' | sort -u`, then `task go:vuln 2>&1 | tail -1`
Expected on this machine's go1.27.2 (measured 2026-10-28 on the final Tasks 3-7 code with the Task 8 bump): exit 3, with exactly one ID, `GO-2026-6809` (`crypto/x509`, found in go1.27.2, fixed in go1.27.3). It is an **environment residual**: the standard library of the local Go, not `go.mod`. CI runs go1.26.10 (Task 8), where it is fixed. Task 8 Step 6 already scanned under go1.26.10 and got `No vulnerabilities found.`; repeat that here:

Run: `GOTOOLCHAIN=go1.26.10 govulncheck ./... > /dev/null; echo "exit=$?"`
Expected: `exit=0`.

If `go version` already prints go1.27.3 or later, the first command prints no ID and `task go:vuln` exits 0. Any ID other than `GO-2026-6809` → stop and report. The other seven IDs that were reachable at base (`GO-2026-6617`, `-6612`, `-6611`, `-6610`, `-6603`, `-6505`, `-6348`) were cleared by Task 8; their reappearance means Task 8 was reverted or its `go.mod` changed.

The exit 3 is govulncheck's ("vulnerabilities found"), which `task` reports as the last line, `task: Failed to run task "go:vuln": exit status 3`; `task`'s own process exit status is `201`. Both are expected for the one residual. The govulncheck footer also lists findings that "your code doesn't appear to call" (`GO-2026-6443`, `-6866`, `-6860`, `-5932`); they are not reachable and do not gate.

- [ ] **Step 2d: Unit tests**

Run: `task go:test`
Expected: exit 0. It runs `go test ./... -race` without the `integration` tag, so `internal/controller` reports `[no test files]`, and `internal/agent`, `internal/ipam` and `internal/iptables` report `ok`. The integration suite is covered by Task 7 Step 8; it is not part of this gate.

- [ ] **Step 3: Confirm the complexity debt actually fell**

```bash
gocyclo -top 5 internal/controller/ 2>/dev/null || go run github.com/fzipp/gocyclo/cmd/gocyclo@latest -top 5 internal/controller/
```

`Reconcile` was **132** at `be6d4e4`. It measured **97** after old Tasks 1-5 during plan review, and **97** again with every task applied (golangci-lint's gocyclo, 2026-10-26). Task 3's migration lives in `applyOwned` and Task 7's change lives in `updateStatus`, so neither adds to `Reconcile`. If `gocyclo` is not installed and the `go run ...@latest` fallback cannot reach the network, skip this step and record the expected figures instead -- it is informational, not a gate. The number should fall substantially; it will not reach the `.golangci.yml` threshold, and it is not expected to — decomposing `Reconcile` is explicitly out of scope. Put the number under a `## PR evidence` heading in the report you write at the end (a section of that final report, not a separate file). Do not push, and do not create or edit any PR; Phase 5 builds the PR body from that section.

- [ ] **Step 4: Verify the acceptance criteria**

Confirm each against the suite. Write the evidence under the same `## PR evidence` section of your final report: one line per criterion (a)–(c), giving the spec name or file:line plus the matching output line from the Task 7 Step 8 run. Do not push or touch any PR.

- (a) *Desired computed once and diffed against actual* — one `ForWireguard` call per object per reconcile, replacing five. The diff is server-side; there is deliberately no client-side comparison.
- (b) *Changing any pod-affecting spec field triggers an update* — covered by the `spec.agent.resources` spec (Task 4) and the sidecar-removal spec (Task 7). **Known exception: `spec.metric` is rendered nowhere; tracked as #50 and out of scope here.**
- (c) *No update churn* — covered by the churn spec (Task 6), asserting both `resourceVersion` stability and writes issued.

- [ ] **Step 5: Commit**

```bash
git add docs/ROADMAP.md .golangci.yml
git commit -m "docs: correct the Phase 0.2 drift-check description and the lint ratchet note

The cited line numbers predated PRs #39 and #49. Replaced with a description
that cannot go stale, plus a pointer to the design document.

The .golangci.yml ratchet comment claimed issue #2 replaces the reconcile
loop's complexity hits wholesale. It replaces only Reconcile's drift checks
(132 -> 97) and leaves updateWireguardPeers alone by design; the comment now
says so.

Refs #2"
```

Then confirm the commit is signed (Global Constraints):

Run: `git cat-file -p HEAD | grep -c '^gpgsig -----BEGIN SSH SIGNATURE-----'`
Expected: `1`. A missing signature means signing silently failed: stop and report BLOCKED (commit signing unavailable).

---

## Self-Review Notes

> **Amended 2026-10-26** after the design gate (findings B1, S1, S2, S6, M1, M5; owner rulings binding). A new Task 3 was inserted and old Tasks 3–7 became 4–8. **Task sections 1 and 2 were not edited beyond one forward note each**, because Task 1 is committed and Task 2 is staged. Inside those two sections, "Task 3" still means today's Task 4 and "Task 4" today's Task 5; the notes say so. The historical defect list below keeps the numbering it was written in, marked where it matters.

> **Amended 2026-10-27** after the plan gate (findings B2–B9, S2–S6, M1–M8). Header and Global Constraints rewritten; `Kind:` lines added; Task 1 and Task 2 gained only the commit-signature check after their commit blocks; Task 3 gained Step 0; Task 6 gained Step 3b and Task 7 Step 4b (mutation checks); Task 8 (now Task 9) Step 2 split into 2–2d. Old step numbers are unchanged, so ledger references to them stay valid.

> **Amended 2026-10-28** after measuring the dependency bump. A new **Task 8** (dependency and toolchain bumps, `go.mod`/`go.sum` only) was inserted before the documentation task, which is now **Task 9**. Tasks 1–7 are unchanged. Task 9's vuln step now expects one environment residual, `GO-2026-6809`, instead of the 8 base IDs. Entries above dated 2026-10-26 and 2026-10-27 that say "Task 8" meant the documentation task and were relabelled; the other numbering in them is as of their date.

**Task list and dependencies.**

| Task | Delivers | Depends on |
|---|---|---|
| 1 | Port identity preserved at three test mangle sites (**done**, `e7d4579`) | — |
| 2 | `applyOwned` seam, `FieldManager`, `failDegraded` (**staged**) | 1 (green suite) |
| 3 | Legacy field-manager migration and M1 error wrapping in `applyOwned` | 2 (edits `apply.go`/`apply_test.go`, uses its spec helpers) |
| 4 | Deployment build-then-apply; Task 1's gofmt defect fixed | 2, 3 (calls `applyOwned`, `failDegraded`) |
| 5 | Both Services build-then-apply | 1 (port identity), 2, 3 |
| 6 | Counting client, no-churn spec, zero-migration assertion | 3 (JSON-patch migration is what it counts separately), 4, 5 (the applies it counts) |
| 7 | Six convergence specs; `Degraded` cleared in the `Ready=True` write | 3 (adoption specs), 4, 5 (applies), 6 (suite wiring already in place) |
| 8 | `go.mod`/`go.sum` bumps (x/net, grpc, otel, toolchain go1.26.10) clearing the reachable vulnerabilities; no Go source changes | 7 (committed; the integration suite is re-run on the final code) |
| 9 | ROADMAP and `.golangci.yml` comment fixes; `task ci` | 1–8 |

**Spec coverage.** Every section of the design maps to a task: §3.1 → Tasks 2 and 3 (wrapped errors, migration); §3.2 → Task 4; §3.3 → Task 5; §3.4 → Task 2 (`failDegraded`), Task 7 Step 5 (its spec), and Task 7 Steps 5–7 (*Clearing `Degraded`*); §3.5 → Task 5 Step 3; §3.6 → Task 4 Step 4; §6.1 → Task 3 (unit specs) and Task 7 Step 4 (end-to-end); §7.1 → Task 6; §7.2 → Task 1 (all three port-mangle sites) and Task 4 Step 6; §7.3 co-ownership rule → Global Constraints, applied in Task 7's preamble; §7.3 specs 1–7 → Tasks 4, 5, 6, 7 (spec 1 → Task 6; spec 2 → Task 4; spec 3 → Task 5 and Task 7 Step 1; spec 4 → Task 7 Step 2; spec 5 → Task 7 Step 3; spec 6 → Task 7 Step 4; spec 7 → Task 7 Step 5); §8 documentation → Task 9; §9 → Task 9 Step 4, recorded as a tracked exception; §10.2 item 10 → Task 7 Step 4 (Service variant); §10.2 item 11 → Task 3 Step 1 and Task 7 Step 4 (generation and leftover-entry assertions); M5 → Task 9 Step 1b.

**Disclosed divergences from the design, and from this plan's own Task 2 text.** Eight, all deliberate (item 8 is at the end of this list):

1. **`failDegraded` writes `ConditionDegraded`/`True`, not the design's `ConditionReady`/`False`.** The design's snippet was a bug: `updateStatus`'s switch (`wireguard_controller.go:141-148`) maps `ConditionReady` to `Status.Status = Ready` regardless of the condition's own `Status` field, so an apply failure would have reported the object as *ready*. The design has been corrected to match this plan.
2. **`failDegraded` logs a status-write failure rather than combining it with `errors.Join`.** The status write is best-effort and retried next reconcile, and the caller must see the apply error unchanged. *(Corrected 2026-10-26, finding S2: this note previously claimed wrapping would defeat `errors.IsConflict`-style checks. It would not. `apimachinery@v0.30.3/pkg/api/errors/errors.go:810` uses `errors.As`, so `%w` and `errors.Join` are transparent to them, and Task 3's wrapped errors rely on exactly that.)*
3. **Build errors route through `failDegraded` too.** The design used a plain `log.Error` for `ForWireguard` failures; this plan gives them `DeploymentBuildFailed` / `ServiceBuildFailed` reasons for consistency.
4. **Task 1 takes the design's fallback option** (preserve port identity in place) rather than its stated preference (delete the helper and its call sites). Reason recorded in Task 1: three lines versus eight call sites, and identity-preservation is already the established pattern in this file.
5. **The Service variant of spec 6 relaxes design §7.3's rule.** §7.3 allows only spec 6 to `Create` an operator-owned object, and requires it to exercise a removal. Task 7 Step 4's Service variant also `Create`s one (the legacy `vpn-svc`) and exercises no removal. Instead it asserts the seeded legacy entry owns neither `f:clusterIP` nor `f:nodePort` and that both allocations survive adoption, which is what §10.2 item 10 asks. The rule's purpose (a test-client co-owner masking a removal) does not arise: the seed is owned by `manager`, which the migration removes, and the spec asserts `manager` is gone before it reads the allocations.
6. **`applyOwned`'s read-error text differs from design §3.1.** The design's snippet wraps a failed pre-apply `Get` as `read %s %s before apply: %w`; Task 3 uses `read %s %s for field-manager migration: %w`, which names why the read exists. The other three messages match the design. No spec asserts this text (the path is untested; see Known risks).
7. **Task 2's staged `apply_test.go` differs from Task 2 Step 1's code block in one place.** The staged `failDegraded` spec re-issues `r.failDegraded(...)` on every `Eventually` poll, with a comment explaining why: `updateStatus` swallows a `Conflict` and leaves the retry to the next reconcile, so a single call can be lost to the manager's concurrent status write. Step 1's block shows the earlier single-call form. The staged file is authoritative; Step 1's block is left as the record of what was planned.
8. **Task 8 (dependency bumps) is not in the design's scope (§8).** It was added by the repo owner's ruling in the Run contract (*Known vulnerabilities*, answered 2026-10-27: fix in this branch, do not defer), after the plan gate found 8 reachable vulnerabilities already present at base. It changes only `go.mod`/`go.sum` and moves no `k8s.io/*` or controller-runtime version.

**Type consistency.** `applyOwned(ctx, obj) error` keeps its signature through Task 3 and is called with it in Tasks 4 and 5 and in the specs of Tasks 2 and 3. `failDegraded(ctx, req, err, reason, msg) (ctrl.Result, error)` is used with that exact signature in Tasks 4 and 5. `FieldManager` is referenced in Tasks 2, 3 and 7 (not Task 6). `legacyFieldManagers` is defined and used only in Task 3; specs use the literal `"manager"` on purpose. `writeCounter` is defined in Task 6 Step 2 and used in Task 6 Step 1, and both land in the same commit. `countingClient`'s methods are `writesTo`, `migrationsTo` and `reset`. The Interfaces block previously said `writesFor(namespace string) int`, which no code defined; corrected.

**Verification of the 2026-10-26 amendment.** Every code block added or changed was executed in a scratch worktree at `e7d4579`, with the staged Task 2 files copied in and Tasks 3–7 and 9 applied as written (the documentation task was Task 8 then; it is Task 9 now):
- Task 3 RED from the staged code: `4 Passed | 2 Failed` with the two expected messages. Task 3 GREEN: `29 Passed | 0 Failed` under `-race`, Task 2's four specs included. `go mod tidy` left `go.mod`/`go.sum` unchanged.
- Final state: `go build ./...`, `go vet ./...` and `go vet -tags=integration ./internal/controller/` clean; `go test ./... -race` green; integration suite `38 Passed | 0 Failed` under `-race`; `golangci-lint run ./...` → `0 issues.` (with the toolchain workaround in Task 9 Step 2a).
- Task 7 RED without the S1 change: `37 Passed | 1 Failed`, only the recovery spec.
- Mutation check: with `legacyFieldManagers` set to an unused name, the Task 3 migration spec, spec 6 and its Service variant all fail. Spec 6 fails at the removal with `["agent", "wstunnel"]`.
- Task 7 Steps 1–3 and the failure half of Step 5 had never been executed before this amendment; they pass unchanged.

**Verification of the 2026-10-27 amendment.** Run in a scratch worktree at `/private/tmp/gate2fix-issue2` (`e7d4579`, staged Task 2 files copied in, Tasks 3–7 applied from this file's code blocks):
- Task 3 Step 2 RED: the three `<[]string` lines now quoted there, `4 Passed | 2 Failed`.
- Final state: `go build ./...`, `go vet ./...` and `go vet -tags=integration ./internal/controller/` clean; integration suite `38 Passed | 0 Failed` under `-race`, including the new S2 assertion; `golangci-lint run ./...` → `0 issues.` under the go1.27.1 toolchain.
- Task 6 Step 3b and Task 7 Step 4b: each mutation went red with the quoted output and back to green on revert, at the spec counts each task actually has (32 and 37).
- The Service seed's `manager` entry, printed: `f:metadata.f:labels`, `f:spec` → `f:externalTrafficPolicy`, `f:internalTrafficPolicy`, `f:ports` (`k:{"port":51820,"protocol":"UDP"}`), `f:selector`, `f:sessionAffinity`, `f:type`. No `f:clusterIP`, no `f:nodePort`.
- Task 9 (then numbered 8): `task ci` stops at `go:lint` (typecheck only); `task go:tidy:check` exit 0; `task go:vuln` reported the 8 IDs that were also present at `be6d4e4` (before Task 8 existed); `task go:test` exit 0.

**Verification of the 2026-10-28 amendment (Task 8).** The `go get ... && go mod tidy` command in Task 8 Step 2 was run on a fresh worktree at `e7d4579` and produced a `go.mod` and `go.sum` byte-identical (`cmp`) to the bumped scratch tree; with Tasks 3–7 applied, build, vet (both tag sets), `go test ./... -race`, `go mod tidy -diff` and the integration suite under `-race` pass, lint reports `0 issues.`, and `govulncheck` reports `No vulnerabilities found.` under go1.26.10 and only the local-toolchain residual `GO-2026-6809` under go1.27.2.

**Why `apply.go`'s package comment is one line from Task 3 on.** golangci-lint's revive `package-comments` rule fires on every line of a new file under the `new-from-merge-base` ratchet, and no existing file in this package carries a package comment. The staged Task 2 `apply.go` explained that inside the package comment; Task 3's rewrite keeps the one-line package comment that satisfies the rule and drops the explanation, which describes how the file came to be rather than what it does.

**Known risks carried into execution.**

- Task 7 Step 5 depends on the apiserver rejecting `nodePort` on a ClusterIP Service. Verified at Gate 2 against envtest 1.30.0 (`spec.ports[0].nodePort: Forbidden: may not be used when 'type' is 'ClusterIP'`), and the spec reached `ServiceApplyFailed` on 2026-10-26. The instruction remains to stop and report if it ever does not, rather than weakening the assertion.
- `desiredDep.Status.ReadyReplicas` is exercised by nothing: envtest runs no Deployment controller, so it is always 0 and no spec distinguishes "reads the apply response's status" from "reads a zero value". Carried forward from design §11 as a stated gap, not silently dropped.
- Two error paths in Task 3 are untested: a non-NotFound error from the pre-apply `Get`, and a failed migration `Patch` (for example a 409 from a stale cache read). Neither can be induced in envtest without a fault-injecting client. Both are plain `return fmt.Errorf(...%w...)` and route through `failDegraded` like any apply error. A stale-cache 409 on the migration patch therefore surfaces as a transient `Degraded=True` until the next successful reconcile clears it; this is accepted, not suppressed (Task 3, notes).

**Parked items (not in this plan, by decision or because untestable here).**

- **Objects last written by a differently named binary** (a `go run` dev build) keep their legacy entry and are not migrated (design §6.1, *Known limitation*). The remedy is to delete the object.
- **`Progressing=True` is never cleared** (design §3.4, pre-existing). Out of scope.
- **Design §10.2 item 7**: whether the apiserver preserves an allocated `nodePort` when the operator relinquishes a previously owned non-zero `spec.nodePort` (the user sets it back to 0). Not covered. Task 7's Service variant covers only the case where the operator never rendered a `nodePort`.
- **Design §10.2 item 10** is *included*, not parked, and is now closed in the design (2026-10-27). The Service variant of spec 6 (Task 7 Step 4) measured that the legacy entry owns neither `f:clusterIP` nor `f:nodePort`, and that both are unchanged across adoption on envtest 1.30.0. Other Kubernetes versions remain unverified (§10.2 item 9).
- A future cache transform that strips `managedFields` would make the migration silently compute a nil patch. Documented at the code site in Task 3; no guard spec.

**Defects found and fixed before execution.** Three during self-review, five more by the Gate 2 adversarial review, which executed the plan against a scratch copy of the repo and measured four of them as real compile or test failures, and four more at the 2026-10-26 amendment (items 9–12). **Items 1–8 use the pre-amendment numbering: their "Task 3" to "Task 6" are today's Tasks 4 to 7, and their "Task 7" is today's Task 9.**

Found during self-review:

1. **`apply.go` was given `//go:build integration`.** Only the two `_test.go` files in `internal/controller/` carry that tag; `wireguard_controller.go` does not. A tagged `apply.go` would leave `applyOwned` undefined in the ordinary build, breaking `go build ./...` and the manager binary as soon as Task 3 called it. Tag removed, and Task 2 Step 4 now runs `go build ./...` explicitly to catch a regression.
2. **`churn_test.go` referenced `Timeout` and `Interval`.** Those are consts declared *inside* the `Describe` closure of `wireguard_controller_test.go`, invisible to a `Describe` in another file. The spec now declares `churnTimeout` / `churnInterval` and imports `time`.
3. **The `svcFound = &corev1.Service{}` reset fell between the two replaced ranges in Task 4.** It re-initialises a variable whose declaration Step 3 deletes, and it sits above the point where Step 4 re-declares `svcFound`, so leaving it produces `undefined: svcFound`. Step 3 now deletes it explicitly.

Found by the Gate 2 review, all measured:

4. **Four new specs wrapped `reconcileServiceWithTypeLoadBalancer` in `Eventually`.** That helper asserts internally with a bare `Expect` (`wireguard_controller_test.go:65`), so on the first poll — before the operator has created the Service — it calls the Ginkgo fail handler and aborts the spec instead of returning an error to retry. It also meant Task 3's TDD gate went red for the wrong reason, which an unattended executor would have misread as confirmation. All four now wait for the Service with their own `Eventually`, then call the helper once, matching every existing call site.
5. **Two further port-mangle sites, outside Task 1's original scope.** `:371` passes `5182` — a typo for `51820` — which changes the port's merge key and yields `spec.ports[1].name: Duplicate value: "wireguard"`, breaking the NodePort end-to-end spec. `:838` inlines a copy of the helper's original defect. Task 1 now fixes all three sites and verifies none remain.
6. **The `failDegraded` spec raced the manager's reconciler.** It asserted `Status.Status == Error`, but the manager reconciles every `Wireguard` and `updateStatus` rewrites the summary field from whichever condition it wrote last. The spec now asserts on the Degraded *condition* under `Eventually`, and cleans up the children the manager creates.
7. **The churn spec's premise was false and its ceiling had a margin of one.** `agent.State` embeds the whole `Wireguard` (`Server: *wireguard.DeepCopy()`), so annotating it to trigger a reconcile *is* serialised into `state.json` and legitimately rewrites the peer-config Secret — the spec's comment claimed nothing the operator renders had changed. The counter also tallied every object rather than the three the design required. It now counts per object and asserts a real invariant: the metrics Service is applied exactly once per reconcile by construction, so the Deployment and main Service must match it one-for-one. The pre-change code could write the Deployment four times in a single pass.
8. **golangci-lint findings on the new files were unaddressed.** `revive`'s `package-comments` and `modernize`'s `rangeint` both fire on brand-new files under `new-from-merge-base`. Pre-decided rather than left to the executor: a package comment lives on `apply.go`, and all loops use `for i := range N`.

Found at the 2026-10-26 amendment. Items 9, 11 and 12 were measured or read directly. Item 10 is reasoned from B1 plus the mutation check above; the old spec itself was not run:

9. **Task 1's commit left a gofmt defect** (`wireguard_controller_test.go:62-63`, a missing blank line; `be6d4e4` was clean). Old Task 3 Step 5 expected `gofmt -l internal/controller/` to print nothing, so an unattended builder would have stopped there. Today's Task 4 Step 5 now checks `gofmt -d` and fixes it.
10. **Old Task 6's adoption spec could not detect B1.** It seeded under `legacy-manager` and asserted only that the image converged. A forced apply converges a differing value with or without the migration, so the spec passed against the defect. It is replaced by Task 7 Step 4, which seeds under `manager` and asserts a removal; that removal fails without the migration.
11. **Task 6's Interfaces block named `writesFor(namespace string) int`**, which the code never defined (the code has `writesTo(ns, names...)`). Corrected, and `migrationsTo` added.
12. **Old Task 3 Step 3 would have duplicated the `// deployment` comment.** The replacement block restates it, and the deletion started one line below it. The step now deletes from the comment line.

## Parked gate findings

From the plan gate's second pass (2026-10-28). The pass's one Blocking finding and its Significant S1–S8 were fixed in the plan. These Minor items are recorded here rather than fixed:

1. **Ginkgo `[Fail]` line order (Task 7 Step 4b, Task 3 Step 2).** Ginkgo v1 shuffles top-level containers on each seed, so the `[Fail]` lines can appear **in any order**. Match the set, not the order. The same note is under each affected Expected block.
2. **Churn spec `reset()` timing (Task 6).** `reset()` runs about 250 ms after the Deployment appears. A reconcile that straddles it could make `depWrites` and `metricsWrites` differ by 1. This is reasoned, not observed: it passed in 5 of 5 runs, and in 12 of 12 earlier probe runs. If it flakes, wait for `Ready=True` before `reset()`.
3. **Task 9 Step 1 checkboxes are prose.** The three ROADMAP checkbox lines are named, not quoted. Match them by their text at ROADMAP l.174–176.
4. **`task ci` stage under a rebuilt golangci-lint (Task 9 Step 2).** With a golangci-lint built on go1.27.2, `task ci` passes lint and then stops at `go:vuln` on the local GO-2026-6809 residual. That is the expected residual from Step 2c, not a new failure.
5. **Controller only — ledger boundary lines.** The ledger has no `started-epoch:`, `dispatch-cap:` or `heartbeat-epoch:` lines. Run feature-workflow's initialize block once on resume, before the first dispatch (cap = 60 for 9 tasks).
