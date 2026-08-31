# Declarative Deployment and Service Reconciliation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **For Claude:** REQUIRED EXECUTION WORKFLOW (follow in order):
> 1. `superpowers:using-git-worktrees` — Isolate work in a dedicated worktree
> 2. `superpowers:subagent-driven-development` — Dispatch a fresh subagent per task
> 3. `superpowers:test-driven-development` — All subagents use TDD
> 4. `superpowers:verification-before-completion` — Verify all tests pass per task
> 5. `superpowers:requesting-code-review` — Code review after each task (built in)
> 6. After all tasks: comprehensive code review on full diff from branch point (automatic)
> 7. `superpowers:finishing-a-development-branch` — Complete the branch
>
> Skills carry their own model and effort settings. Do not override them.

**Goal:** Replace five duplicated Deployment drift checks and one Service port-drift check with a single Server-Side Apply seam, so every spec field converges without a bespoke check.

**Architecture:** One helper, `applyOwned`, stamps the GVK from the scheme and issues a Server-Side Apply with a fixed field manager and forced ownership. The Deployment path and both Service paths each become build-then-apply. The apiserver performs the diff; there is deliberately no client-side comparison. Apply errors route through `failDegraded` so a permanent failure surfaces as a Degraded condition instead of silently looping.

**Tech Stack:** Go 1.26 (toolchain go1.26.7) · `sigs.k8s.io/controller-runtime v0.18.5` · `k8s.io/{api,apimachinery,client-go} v0.30.3` · Ginkgo v1 / Gomega · envtest at Kubernetes 1.30.0

**Design:** `docs/designs/2026-08-29-declarative-deployment-reconciliation-design.md` (approved 2026-08-30)
**Issue:** [#2](https://github.com/jacaudi/wireguard-operator/issues/2)
**Base commit:** `be6d4e4`

---

## Global Constraints

Every task's requirements implicitly include this section.

- **Go package name is `controllers`, not `controller`.** The directory is `internal/controller/` but every file in it declares `package controllers`. Getting this wrong produces `found packages controllers and controller`.
- **Build tags: TEST files carry `//go:build integration`; production files do not.** In `internal/controller/`, `suite_test.go` and `wireguard_controller_test.go` start with the tag; `wireguard_controller.go` and `wireguardpeer_controller.go` do not. Both directions were verified empirically:
  - A **new test file without the tag** is *included* in the untagged `go test ./...`, where the suite's symbols do not exist, and fails loudly — `undefined: k8sClient`, `[build failed]`. Give every new test file the tag on line 1 followed by a blank line.
  - A **new production file with the tag** disappears from the ordinary build. `apply.go` tagged `integration` leaves `applyOwned` undefined in `go build ./...`, in `task go:build`, in `govulncheck`, and in the `cmd/manager` binary — the seam would exist only in the test build. Give new production files no tag.

  Note the failure is *latent*: with `apply.go` tagged, Task 2's `go build ./...` still passes, because nothing untagged references the new symbols yet. It breaks in Task 3.
- **`Timeout` and `Interval` are consts declared inside the `Describe("wireguard controller", ...)` closure** (`wireguard_controller_test.go:120-121`), not at package scope. A `Describe` in any other file cannot see them and must declare its own.
- **`errors` in `wireguard_controller.go` is `k8s.io/apimachinery/pkg/api/errors`, not stdlib.** Do not call `errors.Is`, `errors.As`, or `errors.Join` on that identifier. If stdlib errors are needed, import it as `stderrors "errors"`.
- **Conventional commit subjects are mandatory.** release-please parses them; a type-less subject contributes no changelog entry and no version bump. Use `refactor:` for reconcile-loop changes, `test:` for test-only changes, `docs:` for documentation.
- **Do not use `feat!:` or a `BREAKING CHANGE:` footer anywhere in this work.** It would bump 2.11.0 → 3.0.0.
- **Never claim a step passed without running it and reading the output.**
- **Run the integration suite with:**
  ```bash
  KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1
  ```
  Add `-race` before the final commit of each task; CI runs `-race`.
- **Full local gate before the final task's commit:** `task ci`.
- **Do not touch:** `updateWireguardPeers`, the Secret block, the ConfigMap block, or the readiness gates. All are out of scope by design decision.
- **`k8sClient` (used by tests) is a direct, non-cached client.** The reconciler's client is cache-backed. Assertions must use `k8sClient`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `internal/controller/apply.go` *(new)* | The `applyOwned` seam, `FieldManager`, and `failDegraded`. Nothing else. | 2 |
| `internal/controller/apply_test.go` *(new)* | envtest coverage of the seam in isolation. | 2 |
| `internal/controller/wireguard_controller.go` | Deployment and Service paths become build-then-apply. | 3, 4 |
| `internal/controller/wireguard_controller_test.go` | Port-identity fixes at three sites; new convergence specs. | 1, 3, 4, 6 |
| `internal/controller/churn_test.go` *(new)* | Counting client and the no-churn spec. Isolated because it needs its own namespace and its own reconciler wiring. | 5 |
| `docs/ROADMAP.md` | Stale line numbers in § Phase 0.2. | 7 |

---

## Task 1: Fix all three Service port-mangle sites (blocking prerequisite)

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

---

## Task 2: The `applyOwned` seam

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

---

## Task 3: Deployment path becomes build-then-apply

**Files:**
- Modify: `internal/controller/wireguard_controller.go` — the block from the `-dep` `Get` through the scheduling-drift check (`:829`–`:921` at `be6d4e4`)
- Modify: `internal/controller/wireguard_controller.go` — the status block's Deployment read (`:962`–`:975`)
- Modify: `internal/controller/wireguard_controller_test.go` — one new spec

**Locate the block by these strings, not by line number:**
- start: the `deploymentFound := &appsv1.Deployment{}` declaration followed by a `Get` for `wireguard.Name + "-dep"`
- end: the closing brace of the block introduced by `// Ensure scheduling settings remain in sync with spec updates.`

**Interfaces:**
- Consumes: `applyOwned`, `failDegraded` (Task 2); `r.deploymentBuilder.ForWireguard(wg) (*appsv1.Deployment, error)`
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
			// reconcileServiceWithTypeLoadBalancer asserts internally with a bare Expect, so it
			// must NOT be wrapped in Eventually -- on the first poll, before the operator has
			// created the Service, that Expect calls the Ginkgo fail handler and aborts the
			// spec rather than returning an error to retry. Wait for the Service to exist,
			// then call the helper once. This is what every existing call site does.
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

Add to the test file's import block:

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

Delete everything from `deploymentFound := &appsv1.Deployment{}` through the closing brace of the scheduling-drift block, and put this in its place:

```go
	// deployment
	//
	// One build, one apply. The apiserver diffs the applied field set against the stored
	// object, so every field the builder renders converges without a bespoke drift check --
	// which is what this replaced: four hand-written checks plus a create branch, all of them
	// rebuilding the same desired Deployment.
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

If that prints nothing, delete `"reflect"` and `"slices"` from the import block. Then:

```bash
go build ./... && gofmt -l internal/controller/
```

Expected: no output from either.

- [ ] **Step 6: Run the tests to verify they pass**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race
```

Expected: PASS, including the new spec and the pre-existing `:1236` tolerations and `:1426` userspace specs, which now converge through the apply instead of through their deleted drift checks.

If the tolerations or userspace specs go red on timing rather than on value, increase only that spec's `Eventually` timeout — do not change its assertions.

- [ ] **Step 7: Commit**

```bash
git add internal/controller/wireguard_controller.go internal/controller/wireguard_controller_test.go
git commit -m "refactor: reconcile the Deployment declaratively via Server-Side Apply

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

---

## Task 4: Service paths become build-then-apply

**Files:**
- Modify: `internal/controller/wireguard_controller.go` — metrics Service block (`:489`–`:516`), the orphaned `svcFound` reset (`:518`), main Service block (`:553`–`:603`). The status block's Service read is deliberately **not** edited: the `svcFound := desiredSvc` alias keeps it correct as-is.
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
	// Applying instead of comparing means annotations, target ports, protocols and the second
	// port in dualMode all converge -- the previous check compared only len(Ports) and
	// Ports[0].Port. Server-assigned fields survive because the builder never renders them:
	// clusterIP and ipFamilies are absent from the applied body, and a nodePort of 0 is dropped
	// by omitempty, so the operator never takes ownership of the apiserver's allocation.
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
git commit -m "refactor: reconcile both Services declaratively via Server-Side Apply

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

---

## Task 5: Counting client and the no-churn spec

**Why isolated:** the suite builds exactly one `WireguardReconciler` in `suite_test.go` for all specs, so a global counter would tally every spec's reconciles. This spec counts writes scoped to its own namespace.

**Files:**
- Create: `internal/controller/churn_test.go`
- Modify: `internal/controller/suite_test.go` — wrap the manager client

**Interfaces:**
- Consumes: nothing from earlier tasks (`churn_test.go` does not reference `FieldManager`)
- Produces: `countingClient` with `writesFor(namespace string) int` and `reset()`; package-level `writeCounter *countingClient`

- [ ] **Step 1: Write the counting client and the failing spec**

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
}

func newCountingClient(c client.Client) *countingClient {
	return &countingClient{Client: c, writes: map[string]int{}}
}

func (c *countingClient) record(obj client.Object) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes[obj.GetNamespace()+"/"+obj.GetName()]++
}

// writesTo returns the total writes issued to the named objects in a namespace.
func (c *countingClient) writesTo(ns string, names ...string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for _, n := range names {
		total += c.writes[ns+"/"+n]
	}
	return total
}

func (c *countingClient) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = map[string]int{}
}

func (c *countingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.record(obj)
	return c.Client.Update(ctx, obj, opts...)
}

func (c *countingClient) Patch(ctx context.Context, obj client.Object, p client.Patch, opts ...client.PatchOption) error {
	c.record(obj)
	return c.Client.Patch(ctx, obj, p, opts...)
}

func (c *countingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.record(obj)
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

		// Let convergence finish before measuring.
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

Two failure modes, with different meanings:

- **`Consistently` fails** — the reconciler is persisting a Deployment write on every pass. Capture the two differing `resourceVersion` values and diff the live object against the builder output. **Do not relax the assertion**; a real churn bug is exactly what this spec exists to catch.
- **`depWrites` does not equal `metricsWrites`** — some object is being applied more than once per reconcile. If the Deployment is higher, a second apply call site has crept in. Do not adjust the numbers to match; find the extra call.

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

Refs #2"
```

---

## Task 6: Convergence specs — removal, server-assigned fields, adoption, degraded

**Files:**
- Modify: `internal/controller/wireguard_controller_test.go`

**Interfaces:**
- Consumes: everything from Tasks 2–4
- Produces: no new exported symbols

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
			// reconcileServiceWithTypeLoadBalancer asserts internally with a bare Expect, so it
			// must NOT be wrapped in Eventually -- on the first poll, before the operator has
			// created the Service, that Expect calls the Ginkgo fail handler and aborts the
			// spec rather than returning an error to retry. Wait for the Service to exist,
			// then call the helper once. This is what every existing call site does.
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
			// reconcileServiceWithTypeLoadBalancer asserts internally with a bare Expect, so it
			// must NOT be wrapped in Eventually -- on the first poll, before the operator has
			// created the Service, that Expect calls the Ginkgo fail handler and aborts the
			// spec rather than returning an error to retry. Wait for the Service to exist,
			// then call the helper once. This is what every existing call site does.
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

- [ ] **Step 4: Add the adoption spec**

Covers the upgrade path from an installation whose objects were written by `Create`/`Update`.

> **Important:** `BeforeEach` deletes Wireguards, peers, Services, Nodes, Secrets and ConfigMaps but **not Deployments**, and envtest runs no garbage collector, so `vpn-dep` leaks between specs. This spec must delete it and wait for the delete to land before seeding its own.

```go
		It("adopts a Deployment previously created without a field manager", func() {
			deploymentKey := types.NamespacedName{Name: wgName + "-dep", Namespace: wgNamespace}

			// The suite leaks Deployments between specs; start from a clean slate.
			stale := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deploymentKey.Name, Namespace: deploymentKey.Namespace},
			}
			_ = k8sClient.Delete(context.Background(), stale)
			Eventually(func() bool {
				dep := &appsv1.Deployment{}
				err := k8sClient.Get(context.Background(), deploymentKey, dep)
				return err != nil
			}, Timeout, Interval).Should(BeTrue(), "leftover Deployment should be gone")

			// Seed the pre-change shape: a plain Create, owned by an Update-operation manager.
			labels := map[string]string{"app": "wireguard", "instance": wgName}
			replicas := int32(1)
			seeded := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: deploymentKey.Name, Namespace: deploymentKey.Namespace, Labels: labels,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{MatchLabels: labels},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec: corev1.PodSpec{Containers: []corev1.Container{{
							Name: "agent", Image: "registry.invalid/stale-agent:v0",
						}}},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), seeded, client.FieldOwner("legacy-manager"))).Should(Succeed())

			created := &v1alpha1.Wireguard{
				ObjectMeta: metav1.ObjectMeta{Name: wgKey.Name, Namespace: wgKey.Namespace},
			}
			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())

			serviceKey := types.NamespacedName{Name: wgName + "-svc", Namespace: wgNamespace}
			// reconcileServiceWithTypeLoadBalancer asserts internally with a bare Expect, so it
			// must NOT be wrapped in Eventually -- on the first poll, before the operator has
			// created the Service, that Expect calls the Ginkgo fail handler and aborts the
			// spec rather than returning an error to retry. Wait for the Service to exist,
			// then call the helper once. This is what every existing call site does.
			Eventually(func() error {
				return k8sClient.Get(context.Background(), serviceKey, &corev1.Service{})
			}, Timeout, Interval).Should(Succeed())
			Expect(reconcileServiceWithTypeLoadBalancer(serviceKey, "localhost")).Should(Succeed())

			// Forced apply must take ownership from the legacy manager and converge the image.
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
			}, Timeout, Interval).Should(Equal(wgTestImage), "adoption should converge, not conflict")

			live := &appsv1.Deployment{}
			Expect(k8sClient.Get(context.Background(), deploymentKey, live)).Should(Succeed())
			var managers []string
			for _, mf := range live.ManagedFields {
				managers = append(managers, mf.Manager)
			}
			Expect(managers).Should(ContainElement(FieldManager))
		})
```

- [ ] **Step 5: Add the permanent-failure spec**

```go
		It("reports Degraded instead of looping when an apply can never succeed", func() {
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
		})
```

> If the apiserver in envtest accepts `nodePort` on a ClusterIP Service rather than rejecting it, this spec cannot be written this way. In that case **stop and report** rather than weakening the assertion — the design's §3.4 premise would need revisiting.

- [ ] **Step 6: Run the full suite**

```bash
KUBEBUILDER_ASSETS=$(setup-envtest use 1.30.0 -p path) go test ./internal/controller/ -tags=integration -count=1 -race
```

Expected: PASS, all specs.

- [ ] **Step 7: Commit**

```bash
git add internal/controller/wireguard_controller_test.go
git commit -m "test: cover removal, external drift, adoption and permanent failure

Five specs the old design's suite could not have: sidecar removal (the case
DeepDerivative cannot detect), reversion of a field changed by another field
manager, survival of the apiserver's clusterIP and nodePort allocations,
adoption of a Deployment created before the field manager existed, and a
Degraded condition on an apply that can never succeed.

Refs #2"
```

---

## Task 7: Documentation and final verification

**Files:**
- Modify: `docs/ROADMAP.md` — § Phase 0.2

- [ ] **Step 1: Correct the stale line numbers in the roadmap**

`docs/ROADMAP.md` § Phase 0.2 cites `:824` / `:844` / `:861` / `:871`, which predate PRs #39 and #49. Find them:

```bash
grep -n "824\|844\|861\|871" docs/ROADMAP.md
```

Replace that list with a description that cannot go stale:

```markdown
The Deployment was rebuilt only when one of four hand-written checks fired — agent image drift,
userspace flag toggle, wstunnel sidecar toggle, and nodeSelector/tolerations change — plus a
create branch. All five rebuilt the same desired Deployment. Replaced with a single Server-Side
Apply through `applyOwned`; see
`docs/designs/2026-08-29-declarative-deployment-reconciliation-design.md`.
```

The three acceptance checkboxes immediately below that paragraph (`docs/ROADMAP.md:174-176`) are still unticked. **Tick the first and third; leave the second unticked** and append to it ` — except spec.metric, tracked as #50`. Leaving all three unticked under a past-tense paragraph would make the roadmap contradict itself.

- [ ] **Step 2: Run the full local gate**

```bash
task ci
```

Expected: all stages pass. The golangci-lint ratchet (`new-from-merge-base: origin/main`) will now report on the lines this work touched — that is the intended effect of paying the complexity debt down. Fix anything it reports on **changed** lines.

If golangci-lint emits `Can't process results by diff processor: can't prepare diff by revgrep`, the merge base is missing locally and it is reporting the whole tree (~113 pre-existing findings). Run `git fetch origin main` and retry — do not "fix" the pre-existing findings.

- [ ] **Step 3: Confirm the complexity debt actually fell**

```bash
gocyclo -top 5 internal/controller/ 2>/dev/null || go run github.com/fzipp/gocyclo/cmd/gocyclo@latest -top 5 internal/controller/
```

`Reconcile` was **132** at `be6d4e4` and measured **97** after Tasks 1-5 during plan review. If `gocyclo` is not installed and the `go run ...@latest` fallback cannot reach the network, skip this step and record the expected figures instead -- it is informational, not a gate. Record the number you get in the PR body. It should fall substantially; it will not reach the `.golangci.yml` threshold, and it is not expected to — decomposing `Reconcile` is explicitly out of scope.

- [ ] **Step 4: Verify the acceptance criteria**

Confirm each against the suite, and paste the evidence into the PR body:

- (a) *Desired computed once and diffed against actual* — one `ForWireguard` call per object per reconcile, replacing five. The diff is server-side; there is deliberately no client-side comparison.
- (b) *Changing any pod-affecting spec field triggers an update* — covered by the `spec.agent.resources` spec (Task 3) and the sidecar-removal spec (Task 6). **Known exception: `spec.metric` is rendered nowhere; tracked as #50 and out of scope here.**
- (c) *No update churn* — covered by the churn spec (Task 5), asserting both `resourceVersion` stability and writes issued.

- [ ] **Step 5: Commit**

```bash
git add docs/ROADMAP.md
git commit -m "docs: correct the Phase 0.2 drift-check description

The cited line numbers predated PRs #39 and #49. Replaced with a description
that cannot go stale, plus a pointer to the design document.

Refs #2"
```

---

## Self-Review Notes

**Spec coverage.** Every section of the design maps to a task: §3.1 → Task 2; §3.2 → Task 3; §3.3 → Task 4; §3.4 → Task 2 (`failDegraded`) and Task 6 Step 5 (its spec); §3.5 → Task 4 Step 3; §3.6 → Task 3 Step 4; §7.1 → Task 5; §7.2 → Task 1 (all three port-mangle sites) and Task 3 Step 6; §7.3 specs 1–7 → Tasks 3, 4, 5, 6; §8 documentation → Task 7; §9 → Task 7 Step 4, recorded as a tracked exception.

**Disclosed divergences from the design.** Four, all deliberate:

1. **`failDegraded` writes `ConditionDegraded`/`True`, not the design's `ConditionReady`/`False`.** The design's snippet was a bug: `updateStatus`'s switch (`wireguard_controller.go:141-148`) maps `ConditionReady` to `Status.Status = Ready` regardless of the condition's own `Status` field, so an apply failure would have reported the object as *ready*. The design has been corrected to match this plan.
2. **`failDegraded` logs a status-write failure rather than combining it with `errors.Join`.** The caller needs the original error unwrapped — wrapping would defeat the `errors.IsConflict`-style checks used elsewhere in the file — and the status write is retried next reconcile anyway.
3. **Build errors route through `failDegraded` too.** The design used a plain `log.Error` for `ForWireguard` failures; this plan gives them `DeploymentBuildFailed` / `ServiceBuildFailed` reasons for consistency.
4. **Task 1 takes the design's fallback option** (preserve port identity in place) rather than its stated preference (delete the helper and its call sites). Reason recorded in Task 1: three lines versus eight call sites, and identity-preservation is already the established pattern in this file.

**Type consistency.** `applyOwned(ctx, obj) error` and `failDegraded(ctx, req, err, reason, msg) (ctrl.Result, error)` are used with those exact signatures in Tasks 3, 4 and 6. `FieldManager` is referenced in Tasks 2 and 6 (not Task 5). `writeCounter` is defined in Task 5 Step 2 and used in Task 5 Step 1 — both land in the same commit.

**Known risks carried into execution.**

- Task 6 Step 5 depends on the apiserver rejecting `nodePort` on a ClusterIP Service. Verified at Gate 2 against envtest 1.30.0 (`spec.ports[0].nodePort: Forbidden: may not be used when 'type' is 'ClusterIP'`). The instruction remains to stop and report if it ever does not, rather than weakening the assertion.
- **Task 6's five specs have never been executed.** Gate 2 stopped at Task 5 because Tasks 2 and 5 did not compile as written, and authoring the fixes would have made the reviewer a co-author. Their `reconcileServiceWithTypeLoadBalancer` defect was inferred from the mechanism measured in Task 3, not observed. Expect Task 6 to need more iteration than Tasks 1-5.
- `desiredDep.Status.ReadyReplicas` is exercised by nothing: envtest runs no Deployment controller, so it is always 0 and no spec distinguishes "reads the apply response's status" from "reads a zero value". Carried forward from design §11 as a stated gap, not silently dropped.

**Defects found and fixed before execution.** Three during self-review, five more by the Gate 2 adversarial review, which executed the plan against a scratch copy of the repo and measured four of them as real compile or test failures.

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
