# Design: Declarative Deployment and Service reconciliation

- **Issue:** [#2](https://github.com/jacaudi/wireguard-operator/issues/2) — `[P0] Replace trigger-based Deployment drift checks with declarative reconciliation`
- **Status:** Approved 2026-08-30 (Gate 1 findings folded in; §10.1 closed). Revised 2026-10-26
  (see note below); §10.2 open.
- **Date:** 2026-08-29
- **Base commit:** `be6d4e4`
- **Depends on:** #1 (closed by PR #39, which consolidated rendering onto `internal/resources`)
- **Unblocks:** #5 (`spec.initContainers` / `spec.extraContainers`), #6 (`spec.volumes`), #7 (reserved-name validation)

> **Anchoring note.** Line numbers below are correct at `be6d4e4` and are a *secondary* hint only.
> Issue #2's own line numbers went stale across PRs #39 and #49; this document will too. Every
> in-scope block is therefore also identified by its function or by a distinctive string, and an
> implementation plan derived from this must locate blocks by those strings, not by line number.

> **Revision 2026-10-26 (design gate, Fable 5.1).** An adversarial review against the pinned
> library source found that removal does not converge on objects created before this change.
> §6.1 adds a legacy field-manager migration inside `applyOwned`, chosen by the repository owner;
> §3.1, §3.4, §5.2, §7 and §9 are corrected to match the review and the owner's rulings, and
> what the review could not verify is listed in §10.2. Repo line numbers remain those at
> `be6d4e4` unless another revision is named (commit `e7d4579`, or the staged `apply.go`).

---

## 1. Problem

`internal/controller/wireguard_controller.go` never reconciles the Deployment declaratively. It
rebuilds and writes it only when one of four hand-written checks fires:

| what drifts | guard, identified by | line |
|---|---|---|
| agent image | `deploymentFound.Spec.Template.Spec.Containers[0].Image != r.AgentImage` | `:850` |
| userspace flag toggle | `if existingUserspace != desiredUserspace` | `:874` |
| wstunnel sidecar toggle | `if hasWstunnel != wireguard.Spec.Tunnel.Enabled` | `:895` |
| nodeSelector / tolerations | `// Ensure scheduling settings remain in sync with spec updates.` | `:909` |
| *(create path)* | `if err != nil && errors.IsNotFound(err)` after the `-dep` Get | `:830` |

All five sites execute the identical two steps — `r.deploymentBuilder.ForWireguard(wireguard)`
followed by a write. That is one piece of knowledge ("the desired Deployment") duplicated five
times. Any spec field without its own bespoke check silently does nothing after first create.

The Service has the same disease in a smaller form, in the block commented
`// Update service ports if tunnel configuration changed.`: it compares `len(Ports)` and
`Ports[0].Port` only, so a change to `Ports[1]`, to a target port, to a protocol, or to
`spec.serviceAnnotations` never takes effect.

### Secondary defects the current shape causes

Consequences of the five-call-site structure, fixed by the rewrite rather than separately:

1. **Stale-read cascade.** All four checks compare against the `deploymentFound` fetched once at
   `:830`. After the first check writes, `deploymentFound` is stale, so a reconcile where two
   conditions drift issues two full writes.
2. **Blind overwrite.** The `dep` passed to `r.Update` is freshly built and carries no
   `resourceVersion`, so each of those updates is an unconditional overwrite with optimistic
   concurrency disabled.
3. **Index-0 assumption.** `:850` reads `Containers[0].Image` and assumes the agent is first. It
   is today; nothing enforces it, and #5 makes it false. *(Note: the status block at `:968` already
   resolves the agent by name — this defect exists only at `:850`, and is fixed by that line's
   deletion. No new by-name lookup is required anywhere.)*
4. **Status read from stale data.** The status block at `:962`–`:975` reads `deploymentFound`,
   which by then may be several writes out of date.

---

## 2. Decision

**Reconcile the Deployment and both Services with Server-Side Apply**, through a single helper
that owns the field-manager identity, the GVK requirement, and the one-time migration of field
ownership away from the pre-SSA manager (§6.1).

**How this satisfies criterion (a).** #2's Scope says *"Compute the desired Deployment once,
semantically diff it against the actual, and update when they differ."* Under SSA the diff is
performed **server-side, by the apiserver, against the operator's declared field set** — there is
deliberately no client-side comparison to point at. An executor verifying criterion (a) literally
would look for a `DeepEqual` and not find one; that absence is the design, not an omission. The
"compute desired once" half is satisfied literally: one `ForWireguard` call per object per
reconcile, replacing five.

Chosen over full-replace `controllerutil.CreateOrUpdate` after both were measured against a live
API server (§4). Both converge; the decision rests on a narrow, explicitly-narrow margin (§5.2).

---

## 3. Design

### 3.1 The seam

One helper in `internal/controller`, used for every operator-owned object:

```go
// FieldManager identifies this operator's Server-Side Apply field manager. It is part of
// the on-cluster contract: changing it orphans field ownership on every existing object
// and leaves stale entries in managedFields. Not a tunable.
const FieldManager = "wireguard-operator"

// legacyFieldManagers names the Update-operation managers that wrote operator-owned objects
// before this change, through Create/Update. The apiserver names such a manager from the first
// User-Agent segment, which client-go derives from os.Args[0]; the production binary is built
// as, and runs as, `manager` (images/manager/Dockerfile). Part of the on-cluster contract,
// like FieldManager. Not a tunable.
var legacyFieldManagers = sets.New("manager")

// applyOwned renders an operator-owned object into the cluster with Server-Side Apply.
//
// What the operator ends up owning is what `json.Marshal` of the built object emits, which
// is NOT the same as "the fields the builder sets" in either direction. Zero-valued structs
// are emitted despite `omitempty` (so the operator owns `spec.strategy`, the agent
// container's `resources`, and the pod template's `creationTimestamp`, none of which the
// builder sets); zero-valued scalars with `omitempty` are dropped (so a `nodePort` of 0 is
// NOT owned, which is what lets the server keep its allocation — see §3.3).
//
// Before the apply, ownership held by a legacy manager is folded into FieldManager's Apply
// entry (§6.1). Without that, fields the legacy manager co-owns are never removed when the
// builder stops rendering them.
//
// SSA requires apiVersion/kind in the request body and a typed object's TypeMeta is empty
// after construction, so the GVK is derived from the scheme and stamped on before the call.
func (r *WireguardReconciler) applyOwned(ctx context.Context, obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, r.Scheme)
	if err != nil {
		return fmt.Errorf("resolve GVK for %T: %w", obj, err)
	}
	obj.GetObjectKind().SetGroupVersionKind(gvk)
	key := client.ObjectKeyFromObject(obj)

	live := obj.DeepCopyObject().(client.Object)
	if err := r.Get(ctx, key, live); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("read %s %s before apply: %w", gvk.Kind, key, err)
		}
		// NotFound: the apply below creates the object SSA-born; nothing to migrate.
	} else {
		patch, err := csaupgrade.UpgradeManagedFieldsPatch(live, legacyFieldManagers, FieldManager)
		if err != nil {
			return fmt.Errorf("compute field-manager migration for %s %s: %w", gvk.Kind, key, err)
		}
		if patch != nil { // nil: no legacy entry left, so no write
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

Imports this adds to the staged `apply.go`: `fmt`, `apierrors "k8s.io/apimachinery/pkg/api/errors"`,
`k8s.io/apimachinery/pkg/types`, `k8s.io/apimachinery/pkg/util/sets` and
`k8s.io/client-go/util/csaupgrade` — all inside modules already pinned in `go.mod`
(`k8s.io/apimachinery v0.30.3`, `k8s.io/client-go v0.30.3`). The `UpgradeManagedFieldsPatch`
signature above is the one at `k8s.io/client-go@v0.30.3/util/csaupgrade/upgrade.go:122`
(`obj runtime.Object, csaManagerNames sets.Set[string], ssaManagerName string, opts ...Option`).
**Status of this snippet against the code:** the staged `internal/controller/apply.go` (plan Task 2)
has the GVK stamp and the forced apply only, returning the `GVKForObject` and `Patch` errors bare;
the migration and the error wrapping land together in the plan's migration task. This snippet and
the §3.4 one were type-checked and vetted on 2026-10-26 in a scratch module built from this repo's
`go.mod`/`go.sum`, against a stub reconciler; that checks the API usage, not the behaviour.

Verified (§4, Q13): `apiutil.GVKForObject` resolves `apps/v1, Kind=Deployment` from the scheme, so
no GVK is hardcoded; and the apply **response is decoded back into `obj`**, returning with `uid`,
`resourceVersion`, `.Status` and `managedFields` populated, at a `resourceVersion` identical to a
subsequent live `Get`.

**Owning an emitted-empty struct is not owning its leaves.** Because the operator applies
`spec.strategy` as `{}`, it owns the struct and nothing inside it. A `kubectl edit` that sets
`strategy.type: Recreate` writes a leaf outside the applied set, raises no conflict, and is **not
reverted**. The agent container's `resources` behaves the same way while `spec.agent.resources` is
unset: a foreign `requests.cpu` survives. Convergence covers the fields the builder renders, not
every field of the object.

**Exclusive ownership needs the migration.** Every removal this design promises assumes the
operator's Apply entry is the only owner of what it renders. That is true of
an object the operator created by apply. It is **not** true of an object written by `Create`/`Update`
before this change until the migration in §6.1 has run on it.

This is the Patchbay seam for the roadmap: adding gluetun, extra containers or volumes is a change
inside the relevant builder plus, at most, one more `applyOwned` call. No sibling touched, no new
drift check ever written again.

**Note for future work:** `applyOwned` cannot be exercised by controller-runtime's fake client,
which rejects `ApplyPatchType` outright (`pkg/client/fake/client.go`). Any test of this seam must
be an envtest. Nothing in the repo uses the fake client today, so this forecloses nothing now.

### 3.2 Deployment path

The block from the `-dep` Get through the scheduling-drift check (`:829`–`:921`, 93 lines)
collapses to:

```go
desiredDep, err := r.deploymentBuilder.ForWireguard(wireguard)
if err != nil {
	log.Error(err, "Failed to build deployment", "wireguard.Name", wireguard.Name)
	return ctrl.Result{}, err
}
if err := r.applyOwned(ctx, desiredDep); err != nil {
	return r.failDegraded(ctx, req, err, "DeploymentApplyFailed",
		"unable to apply deployment")
}
```

`desiredDep` then replaces `deploymentFound` in the status block, which fixes defect 4 at no cost
because the apply response already carries `.Status`. The standalone `Get` is deleted; the only
read left is the one inside `applyOwned` that feeds the migration (§6.1), cache-backed like the
one it replaces.

The create-and-return-early branch disappears: apply is create-or-update in one request, so there
is no create path to special-case and no synthetic requeue.

**API-call accounting, stated correctly.** The deleted `Get` cost *zero* apiserver round-trips —
the manager's client is cache-backed by default. In today's steady state none of the four drift
checks fire, so a no-change reconcile issues *zero* writes. After this change every reconcile
issues **three** unconditional SSA PATCHes (Deployment, main Service, metrics Service). The net is
**+3 apiserver round-trips per reconcile, not −1.** That is an acceptable trade at this operator's
scale — a handful of `Wireguard` objects — and it buys convergence on every field. It is recorded
here because an earlier draft of this document claimed the opposite. The §6.1 migration adds no
round-trip in steady state (its read is served from the cache and its patch is nil); it adds one
write per pre-existing object, once.

### 3.3 Service path

Two Services are involved; both move to `applyOwned`:

- **metrics Service** (`:489`–`:516`) — created once, never reconciled. Becomes an apply. Its
  builder, `ForWireguardMetrics` (`internal/resources/service.go:104`–`:130`), reads only the
  `Wireguard`'s name and namespace and nothing from its spec, so it is as spec-independent as the ConfigMap §8 leaves
  out. It is converted anyway because doing so deletes a create-and-return-early branch and the
  `MetricsServicePending` pass (§3.5), and makes external edits to it revert.
- **main Service** (`:553`–`:603`) — create branch plus the `Ports[0]`-only drift block. Both
  collapse into a single apply.

The readiness gates that follow (LoadBalancer ingress `:620`, NodePort `:640`, ClusterIP `:670`)
are **unchanged**. They read observed state to decide whether to proceed; they are not drift checks.

`svcFound` is replaced by the main Service's apply response; the now-dead metrics `svcFound`
declaration at `:489` and its reset at `:518` are deleted.

#### Server-assigned fields are safe

`spec.clusterIP`, `spec.ipFamilies`, and — when `wg.Spec.NodePort` is 0 — the allocated
`spec.ports[].nodePort` all survive repeated applies untouched, because `omitempty` drops them from
the marshalled body so the operator never owns them (§4 Q9, Q10). This was the largest risk in
bringing the Service into scope and it is measured, not assumed.

#### ServiceType transitions are safe

`wg.Spec.ServiceType` carries no immutability marker, so a user can change it at any time. All five
transitions were measured (§4 Q14): LoadBalancer → ClusterIP correctly drops both `loadBalancerIP`
and the allocated `nodePort`, and `clusterIP` is preserved throughout.

#### Port identity is a merge key — this is load-bearing

`ServiceSpec.Ports` is `+listType=map` with **two** keys, `port` *and* `protocol`
(`k8s.io/api@v0.30.3/core/v1/types.go:5130-5133`). Two consequences:

1. An entry with the same port number but a different protocol is a **distinct** entry. The
   operator's apply of `{name: "wireguard", protocol: UDP, port: 51820}` does not replace a stored
   `{name: "", protocol: TCP, port: 51820}` — it adds alongside it. A multi-port Service requires
   every port to be named, so the unnamed entry then fails validation and the apply returns
   `spec.ports[0].name: Required value` **on every subsequent reconcile, permanently**.
2. This is stricter than today's behaviour. The current `Ports[0].Port`-only check compares
   `len == len` and `51820 == 51820`, concludes nothing changed, and leaves the mangled entry in
   place. SSA is not blind to it.

This is not hypothetical: it is exactly what the existing test helpers produced (§7.2; Gate 1
finding B1). The
production analogue — a foreign writer setting a different protocol on the WireGuard port — is
narrow but real, and is why §3.4 exists.

### 3.4 Error path must surface, not retry invisibly

Two reachable conditions produce a *permanent* apply failure rather than a transient one: the port
collision above, and `spec.nodePort` set together with `serviceType: ClusterIP` (the builder sets
`NodePort: wg.Spec.NodePort` unconditionally at `internal/resources/service.go:53`, and the
apiserver rejects `nodePort` on a ClusterIP Service). The second is pre-existing — today the
`Create` fails identically — but in both cases a bare `log.Error` + `return ctrl.Result{}, err`
leaves the operator retrying forever with a stale `Status.Status` and nothing user-visible.

That retry is not a hot loop. Returning an error requeues the request with rate-limited backoff:
controller-runtime defaults the controller's limiter to `workqueue.DefaultControllerRateLimiter()`
(`controller-runtime@v0.18.5/pkg/controller/controller.go:152`; `SetupWithManager` sets no
`RateLimiter`), which backs off per item exponentially from 5 ms to 1000 s
(`client-go@v0.30.3/util/workqueue/default_rate_limiters.go:39`–`:45`). The problem this section
fixes is invisibility, not CPU.

Every apply error therefore routes through a small helper that records a Degraded condition before
returning. This is the staged `internal/controller/apply.go` (`func (r *WireguardReconciler)
failDegraded`, `:60`–`:75`) verbatim:

```go
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

**Corrected at Gate 2.** An earlier draft of this snippet used `Type: ConditionReady, Status:
metav1.ConditionFalse`. That would have been a bug: `updateStatus`'s switch
(`wireguard_controller.go:141-148`) maps `ConditionReady` to `Status.Status = Ready` regardless of
the condition's `Status` field, so an apply failure would have reported the object as *ready*.
`ConditionDegraded` maps to `Status.Status = Error`, which is the intent.

The same draft combined a status-write failure into the returned error with `errors.Join`. It is
logged instead, because the status write is best-effort and is retried on the next reconcile,
and the caller must see the apply error unchanged. *(Corrected 2026-10-26: an earlier version
justified this by saying wrapping would defeat `errors.IsConflict`-style checks. That was wrong —
`apimachinery@v0.30.3/pkg/api/errors/errors.go:810` resolves the reason with `errors.As`, so `%w`
and `errors.Join` are both transparent to those checks. The conclusion stands; the reason did
not. An earlier snippet here also called an undeclared `log`; the version above declares it.)*

#### Clearing `Degraded`

`updateStatus` upserts by condition type (`meta.SetStatusCondition`, `wireguard_controller.go:138`),
and the success path writes only `Ready=True` (`Reason: "Active"`, `:1028`). Left at that, one
transient apply failure would leave the object carrying `Degraded=True` beside `Ready=True`
indefinitely. The rule:

- **On a reconcile where every apply succeeds, `Degraded` is set to `False`** with reason
  `Reconciled`, alongside `Ready=True`. All three applies precede the `Ready` write, so reaching
  `:1028` is exactly that condition.
- **It must be the same status write as `Ready=True`, not a second `updateStatus` call.**
  `updateStatus`'s switch maps `ConditionDegraded` to `Status.Status = Error` regardless of the
  condition's `Status` (`:144`–`:145`), so a separate `Degraded=False` call would report the object
  as `Error`. A second call would also re-read through the cache, and `updateStatus` swallows a
  `Conflict` (`:152`–`:154`), so the clearing write could be lost without trace. The plan chooses
  the mechanism.
- **Residual, accepted:** a reconcile whose applies succeed but which stops at a readiness gate
  (§3.3) leaves `Degraded=True` until a later reconcile reaches `Ready`. The summary meanwhile reads
  `Pending`, because the gate's `Progressing` write takes the switch's default branch
  (`:146`–`:147`).

**Pre-existing, out of scope: `Progressing` has the same latent bug.** Eight sites write
`Progressing=True` (`:456`, `:506`, `:568`, `:624`, `:642`, `:659`, `:672`, `:819`) and nothing ever
sets it `False`, so a ready object keeps `Progressing=True` beside `Ready=True`. This change
removes two of those writers (§3.5) and otherwise leaves it alone.

### 3.5 Status conditions deliberately dropped

Collapsing the Service create branches removes two `Progressing` conditions that exist only to
describe the gap between "created" and "next reconcile": `MetricsServicePending` (`:506`) and
`ServicePending` (`:568`). With apply there is no such gap — the object exists when the call
returns. No test asserts either reason. **They are dropped deliberately**, and the Degraded
conditions from §3.4 more than replace their diagnostic value.

### 3.6 Status-block correction

`svcFound` and `deploymentFound` in the status block are replaced by the corresponding apply
responses. No status semantics change, and no by-name lookup needs adding — `:968` already does it.

---

## 4. Evidence

Measured against a real API server: envtest at Kubernetes **1.30.0** (the pinned
`ENVTEST_K8S_VERSION`), from a throwaway package **inside this repo's own module**, so the client
libraries were the repo's pins — `sigs.k8s.io/controller-runtime v0.18.5`,
`k8s.io/{api,apimachinery,client-go} v0.30.3` — against this repo's actual builders at `be6d4e4`.
The tree was left clean. Sources and a **complete single-run capture of all sixteen questions** are
in the session scratchpad under `spike-evidence/`; `RESULTS.txt` carries a provenance header
recording the repo commit, the module it ran inside, the four resolved dependency versions, the
control-plane version and the Go toolchain, so the results can be tied to the pins that produced
them. An earlier draft cited a partial capture; that is corrected.

`resourceVersion` values are reported as *stable / not stable* rather than as absolute integers.
The integers are meaningless across runs and an earlier draft quoted them from a different run
than the one archived, which is a documentation defect this revision removes.

| # | Question | Result | Independently re-run at Gate 1 |
|---|---|---|---|
| Q1 | Typed SSA without `TypeMeta`? | **Fails loudly**: `invalid object type: /, Kind=`. With GVK: succeeds. | ✅ reproduced |
| Q2 | Repeated identical apply churns? | **No** — `resourceVersion` stable across 5 applies, `generation` 1, despite the marshal emitting `creationTimestamp: null`, `strategy: {}`, `resources: {}`, `status: {}`. | ✅ reproduced |
| Q3 | Adopting an object previously written by `Create()`? | First apply of *identical* content succeeds **without** force. Forced apply costs exactly one write, then stable. | ✅ reproduced (incl. the conflict error when a field differs) |
| Q4 | SSA removes a container the builder stopped rendering? | **Yes** — wstunnel toggle-off, 2 containers → 1. | ✅ reproduced (also init-container removal) |
| Q4b | Would `DeepDerivative` have caught it? | **No** — returns `true` ("no change"). | ✅ reproduced, source + execution |
| Q5 | Naive `CreateOrUpdate` hot-loops? | **No** — `resourceVersion` and `generation` stable, but a real PUT every reconcile. | — |
| Q6 | Round-trip counts | Both strategies issue a request every reconcile. | — |
| Q7 | External tampering reverted? | **Both strategies revert it.** | — |
| Q8 | Foreign container owned by another manager? | SSA **preserves** (`[agent mesh-proxy]` → `[agent mesh-proxy]`); full-replace **deletes** (`→ [agent]`). | ✅ re-run and captured |
| Q9 | Server-assigned `clusterIP` preserved? | **Yes**, `resourceVersion` stable. | ✅ reproduced |
| Q10 | Allocated `nodePort` preserved? | **Yes**, including one set by a foreign manager. | ✅ reproduced |
| Q11 | Service port-list transitions | All four directions correct, both removals included. | — |
| Q12 | Service annotation removal | Operator-owned dropped; foreign preserved. | — |
| Q13 | Seam viability | GVK derived from scheme; apply response carries uid/rv/status/managedFields; sole manager `wireguard-operator`, operation `Apply`. | ✅ reproduced |
| Q14 | ServiceType transitions (5 of them) | **All safe** — LB→ClusterIP drops `loadBalancerIP` and `nodePort`; `clusterIP` preserved throughout. | ✅ measured at Gate 1 |
| Q15 | `spec.replicas` vs a foreign writer | Forced back to 1. *(Interpretation corrected 2026-10-26: this is a behaviour change, not pre-existing — see below.)* | ✅ measured at Gate 1 |
| Q16 | LB ingress status survives the apply | **Yes**, and is present in the apply response. | ✅ measured at Gate 1 |

Additionally verified from source at Gate 1: the no-op-write short circuit lives in
`k8s.io/apiserver/pkg/storage/etcd3/store.go` inside `GuaranteedUpdate`
(`if !origState.stale && bytes.Equal(data, origState.data)`) — not Deployment-specific, so §5.2's
durability claim holds; and the TypeMeta requirement is enforced server-side in
`apimachinery/pkg/util/managedfields/internal/versioncheck.go`, so it is robust to client-library
version skew.

**What the table does not cover (added 2026-10-26).** Q3 measured the *write cost* of adopting a
`Create`-written object, not which fields the legacy manager keeps afterwards; Q4 measured removal
on an object the operator had created by apply. Neither exercised removal on a legacy object,
which is the case §6.1 shows does not converge without migration. The design gate did not re-run
envtest, and the captures are not in the repo, so these results stand as recorded at Gate 1
(§10.2).

**Q15 is a behaviour change.** Today a foreign `spec.replicas` change persists until one of the
four drift checks happens to fire and rewrites the Deployment. After this change the builder's
`replicas := int32(1)` (`internal/resources/deployment.go:56`, applied at `:70`) is re-asserted
every reconcile. That is consistent with the roadmap's *Single server, not HA* decision
(`docs/ROADMAP.md:39`), and it means **an HPA on this Deployment is unsupported and will be fought
every reconcile.**

---

## 5. Alternatives considered

### 5.1 `equality.Semantic.DeepDerivative(desired.Spec, actual.Spec)` — **rejected on evidence**

The classic pre-SSA operator trick, and the obvious answer to "ignore server-defaulted fields". It
is disqualified by its own source. In
`k8s.io/apimachinery/third_party/forked/golang/reflect/deep_equal.go`, `deepValueDerive` returns
`true` unconditionally when the desired value is a nil or empty slice, map or string. For slices it
returns `false` only if `len(desired) > len(actual)` and otherwise compares positionally over
`len(desired)`; for maps it likewise short-circuits on length and then visits **only the desired
map's keys**.

Consequence: it cannot detect removals. Q4b confirms it reports "no change needed" for the exact
wstunnel toggle-off the current code handles with a bespoke check, and likewise for a tolerations
removal. Adopting it would regress against #2's own second acceptance criterion.

### 5.2 Full-replace `controllerutil.CreateOrUpdate` — **viable, not chosen**

`obj.Spec = desired.Spec` inside a `CreateOrUpdate` mutate function. Expected to hot-loop on
server-defaulted fields; it **does not** (Q5). The apiserver re-defaults the submitted object,
finds the serialized result byte-identical to what is stored, and discards the write — a
storage-layer short circuit, so durable rather than a 1.30 artifact.

It converges, detects removals, and reverts external drift, identically to SSA on every test run.

Not chosen for two reasons, and these are the *only* two:

1. **It is a read-modify-write**, with an optimistic-concurrency conflict path to code and test. A
   single apply has none.
2. **It overwrites foreign field managers on every reconcile** (Q8). What matters is a foreign
   writer on the Deployment object itself. Mesh sidecars are injected at the Pod level and never
   appear in the Deployment. VPA is believed to be the same case — its admission controller
   mutates Pods, not `containers[].resources` on the Deployment template — but that was not
   checked against VPA's documentation (§10.2), so an earlier draft's VPA argument is withdrawn
   rather than restated. The Deployment-level foreign writers that do exist:
   - **`kubectl rollout restart`** writes
     `spec.template.metadata.annotations["kubectl.kubernetes.io/restartedAt"]`. This is where SSA
     concretely wins. The builder sets no template annotations
     (`internal/resources/deployment.go:74`–`:77` sets only `Labels`), so under SSA the annotation
     is outside the applied set and survives. Full-replace of `obj.Spec` wipes it on the next
     reconcile and triggers a second rollout.
   - **HPA** writes `spec.replicas`. The operator owns that field under either strategy and forces
     it to 1 (§4, Q15), so SSA gives no advantage here.
   - **The deployment controller** writes `deployment.kubernetes.io/revision`, on the Deployment's
     own `metadata`, which a `Spec`-only replace leaves alone anyway.
   - **Mutating admission and policy engines** may write anywhere; SSA preserves what they own
     only where the operator does not render the same field (Q8), and that is reasoned rather
     than measured against a real webhook (§11).

   The advantage is real, then, but narrower than a list of tools suggests.

Two arguments *for* SSA were considered and discarded as unsound:

- "SSA removes the read" — **false.** The status block needs the object either way, and per §3.2
  the change *adds* apiserver round-trips rather than removing one. What SSA actually removes is
  the conflict path.
- "SSA is needed for #5/#6" — **false.** Those render user-supplied lists through the operator's
  own builder, so the operator owns them under either strategy.

A reviewer could reasonably prefer 5.2 on KISS grounds; it is the more familiar idiom and needs no
migration. The decision was made by the repository owner with this evidence in hand.

### 5.3 Spec-hash annotation — **rejected**

Store a hash of the rendered spec; rewrite when it differs. Churn-free by construction, but detects
only *operator-intent* change. External drift leaves the hash matching, so the operator never
converges — defeating the issue's purpose. Q7 shows both other alternatives handle that case.

### 5.4 Import upstream defaulting to normalize before comparing — **rejected**

Calling `SetObjectDefaults_Deployment` before a `DeepEqual` would make defaults comparable, but
requires depending on `k8s.io/kubernetes`, which is not published for library consumption and would
drag the control plane into `go.mod`. Disproportionate.

---

## 6. Ownership and migration

Existing objects were written by `Create`/`Update`, so their fields are owned by a manager with
operation `Update`, named from the client's User-Agent. In production that name is **`manager`**:

- the image builds the binary as `manager` and runs `/manager` (`images/manager/Dockerfile:21`,
  `go build … -o manager`; `:30`, `ENTRYPOINT ["/manager"]`);
- `cmd/manager/main.go` sets no `UserAgent` (the `ctrl.NewManager` call at `:93`), so
  controller-runtime fills in `rest.DefaultKubernetesUserAgent()`
  (`controller-runtime@v0.18.5/pkg/cluster/cluster.go:164`–`:165`), whose first segment is
  `filepath.Base(os.Args[0])` (`client-go@v0.30.3/rest/config.go:483`–`:504`);
- the apiserver names the manager from the User-Agent text before the first `/`
  (`managerOrUserAgent` / `prefixFromUserAgent` in `k8s.io/apiserver/pkg/endpoints/handlers/create.go`,
  read at `v0.30.1`, `:258`–`:269` — the nearest version in the module cache; the apiserver is not a
  dependency of this module, so this is not the pinned source).

Under envtest the same derivation yields `controllers.test`, the test binary's name. That name
stays out of production code; spec 6 (§7.3) seeds its legacy object as `manager` instead.

- **A conflict is raised only when the applied value *differs* from a value owned by another
  manager.** Q3: the first apply after a `Create` of identical content succeeds without force.
  The corollary is the defect in §6.1: fields whose value does not differ produce no conflict and
  so are never transferred.
- **`client.ForceOwnership` is nonetheless required** for the first apply that changes a
  previously `Update`-owned field; without it that apply returns a conflict and the operator
  wedges. Verified at Gate 1: the un-forced conflict message is
  `Apply failed with 1 conflict: conflict with "<manager>" using apps/v1: .spec.template.spec.containers[name="agent"].image`.
  Hence the helper passes force unconditionally rather than opportunistically.
- **Force alone does not adopt a legacy object.** An earlier version of this section said adoption
  "costs exactly one extra write per object, once, then stabilises". The write count was measured
  (Q3); the implied convergence was not, and it is false without §6.1.
- **Force-apply is also what makes convergence work** — it takes ownership back from a human who
  ran `kubectl edit`, which is the behaviour #2 wants. It does so because the edit *changed* a
  value the operator renders, which is a conflict and therefore a transfer.
- **Stale owner references rotate correctly.** `ownerReferences` is `+listType=map` keyed on `uid`;
  when a `Wireguard` is deleted and recreated with a new UID, the apply relinquishes the old entry
  and adds the new one with no conflict (verified at Gate 1). This matters because envtest leaks
  Deployments across specs (§7.2).

### 6.1 Legacy field-manager migration

*Added 2026-10-26. Reasoned from source; to be proven by spec 6 (§7.3). Not yet measured in
envtest.*

**The defect.** A forced apply transfers ownership only of fields whose applied value differs from
the stored one. In `sigs.k8s.io/structured-merge-diff/v4@v4.4.1/merge/update.go:108` the conflict
set is `managerSet.Set().Intersection(compare.Modified.Union(compare.Added))`; an unchanged field is
in neither, so the legacy `manager` entry keeps it and the operator becomes a **co-owner**. When
the builder later stops rendering a co-owned field, `prune` removes it and `addBackOwnedItems`
(`update.go:245`–`:247`) puts it back, because another manager still claims it — `addBackDanglingItems`
(`:305`–`:307`) is explicit that items "owned by Updaters … shouldn't be removed". The Kubernetes SSA
reference, as quoted by the review, states the same rule: "each missing item not managed by any other
appliers is removed".

On a pre-existing object, therefore, none of these would ever take effect: turning the wstunnel
sidecar off, setting `tolerations` to null, turning `enableIpForwardOnPodInit` off, deleting a
`serviceAnnotations` key, dropping a dualMode port. Q4 did not see this because it ran on an
object the operator created by apply. Value changes (an image bump, a new port number) are
unaffected: they conflict, and force transfers them.

**The fix: fold the legacy entry into the operator's Apply entry, inside `applyOwned`.** Before the
forced apply, `applyOwned` (§3.1) reads the live object through the manager's cache-backed client
into a fresh `obj.DeepCopyObject()`:

- **NotFound** → skip; the apply creates the object, SSA-born, with no legacy entry.
- **Any other error** → return it, wrapped with context.
- **Found** → `csaupgrade.UpgradeManagedFieldsPatch(live, legacyFieldManagers, FieldManager)`
  (`k8s.io/client-go@v0.30.3/util/csaupgrade/upgrade.go:122`). A non-nil result is sent as
  `client.RawPatch(types.JSONPatchType, patch)`, error wrapped. A nil result means there is no work
  (`:147`–`:151`) and **no write is issued**.
- The existing forced apply then runs unchanged.

It lives in `applyOwned` because that is the one seam all three callers already pass through; no
caller learns that legacy objects exist. `legacyFieldManagers` is a package-level value beside
`FieldManager` in `apply.go`, holding the single name `"manager"`, with a comment citing the
production binary name (`images/manager/Dockerfile`) as its source.

**What the migration does, from source.** For each legacy name, `upgradedManagedFields`
(`upgrade.go:178`–`:239`) converts the most recent `Update` entry with that manager into an `Apply`
entry for `FieldManager`, or — if `FieldManager` already has an `Apply` entry — unions the legacy
field set into it (`unionManagerIntoIndex`, `:244`), then drops every `Update` entry with the legacy
name. Only entries for the main resource are touched (subresource `""`); entries for the `status`
subresource are left alone. The library documents it as working "either before or after initial
server-side apply" (`:72`), which matters for the cache race below. After it the legacy manager
co-owns nothing, so the next forced apply removes whatever the builder no longer emits — unless
some *other* manager also claims the field, which is why §7.3 constrains how tests write.

**Cost.** One extra write per legacy object, once, ever. In steady state the read is served from
the cache (zero round-trips, as for the `Get` it replaces in §3.2) and the patch is nil.

**Concurrency.** The patch replaces `/metadata/managedFields` *and* `/metadata/resourceVersion`
(`upgrade.go:157`–`:171`), with the library's comment that the `resourceVersion` replace makes etcd
reject a stale write with a 409. A stale cache read therefore fails loudly with `Conflict`;
`applyOwned` returns it, `Reconcile` requeues with backoff (§3.4), and the next attempt reads newer
state. A cache that has not yet seen the object at all returns NotFound, the migration is skipped
for that reconcile, the apply succeeds against the live object, and the migration runs on a later
reconcile — the "after initial server-side apply" case.

**Side effect, expected to be benign.** The library warns that a client-side writer "tends to own
more fields (including fields that are defaulted)" and that migration "will possibly remove this
defaults, they will be re-defaulted, that's fine" (`upgrade.go:73`–`:74`). After migration the
operator owns those defaulted fields, its next apply does not render them, and the apiserver
re-defaults them. If the re-defaulted values are identical, the stored spec is unchanged and no
rollout follows. That is reasoned, not measured (§10.2).

**It depends on the cache keeping `managedFields`.** controller-runtime's cache strips them only if
told to (`cache.TransformStripManagedFields`, `controller-runtime@v0.18.5/pkg/cache/cache.go:353`).
Nothing in `cmd/` or `internal/` configures a cache transform today. Adding one would make the
migration silently compute a nil patch on every object, so any future transform must exempt
`managedFields` or move this read off the cache.

**Known limitation.** Only managers named in `legacyFieldManagers` are migrated. An object last
written by a binary with a different name — a `go run` development build, for instance — keeps its
legacy entry, and removals of fields it co-owns still do not take effect on that object. Deleting
the object, so that the operator recreates it by apply, is the remedy.

**No RBAC change is required.** `config/rbac/role.yaml` already grants `patch` on `services`
(line 19, core-group rule) and on `deployments` (line 39, `apps` rule), matching the kubebuilder
markers at `wireguard_controller.go:414` and `:416`. `patch` is the verb SSA uses, and the verb the
migration's JSON patch uses. The migration's read is served by the informers that
`SetupWithManager` already starts for `Owns(&corev1.Service{})` and `Owns(&appsv1.Deployment{})`
(`:1049`, `:1051`), under the `get`/`list`/`watch` grants in the same rules.

---

## 7. Testing

### 7.1 The stated acceptance criterion is insufficient — proposed correction

#2's third criterion reads: *"No update churn when nothing has changed — verified by watching
resourceVersion over several reconciles."*

Q5 shows this **passes for a deliberately wasteful implementation**: full-replace `CreateOrUpdate`
issues a genuine PUT every reconcile, yet `resourceVersion` and `generation` stay pinned because
the apiserver discards the no-op write. A test watching only `resourceVersion` cannot tell a good
implementation from a bad one.

**What a write counter can and cannot do — stated correctly.** A client-side counter counts
*round-trips*, and both strategies issue round-trips every reconcile (Q6: SSA one PATCH per object;
full-replace one GET plus one PUT). **It therefore does not distinguish the two strategies** — an
earlier draft claimed it did, which was wrong. What it *does* catch is the defect this change
actually removes: the stale-read cascade that issues up to four Deployment writes in a single
reconcile. That is the assertion worth having.

Both assertions are therefore included, with these constraints, all of which are blocking for the
implementation plan:

- **Scope the count per object.** `updateWireguardPeers` issues an *unconditional* `r.Update` on
  the peer-configs Secret every reconcile (`:400`–`:403`, no comparison) and `updateStatus` issues
  an unconditional `Status().Update` at `:1028`. Both are out of scope by decision, so a global
  counter can never reach zero. Count writes **to the Deployment and the two Services only**.
- **Guard the counter with a mutex.** CI runs `go test ./... -race -tags=integration`; an unguarded
  `int` written from the reconcile goroutine and read from the test goroutine is a guaranteed
  failure.
- **Solve isolation explicitly.** `suite_test.go:87`–`:92` constructs exactly one
  `WireguardReconciler` with `Client: k8sManager.GetClient()` for the whole suite. Wrapping there
  counts every reconcile from all specs; constructing a second reconciler and calling `Reconcile`
  directly does not isolate anything, because the manager's reconciler is concurrently applying to
  the same objects. **The plan must choose one of:** (a) wrap the manager's client once and expose
  a per-object, resettable counter that a spec zeroes before its measurement window; or (b) run the
  measurement in a dedicated namespace and count only writes whose object namespace matches.
  Option (b) is recommended as it needs no coordination with other specs.

**Steady-state expectation:** after convergence, repeated reconciles of an unchanged `Wireguard`
issue at most one apply per owned object per reconcile — never the four-writes-in-one-reconcile the
current code can produce — and persist zero of them (`resourceVersion` stable). The §6.1 migration
patch is a write to the same objects and must be counted as one, but it occurs only on an object
with a legacy entry; spec 1 measures objects the operator created by apply, so it should see none.

There is a fair counter-reading: at this scale a wasted round-trip is cheap, and `resourceVersion`
is what actually drives watch events and rollouts. Both assertions are kept. **This strengthening
was confirmed on 2026-08-30: #2's third criterion has been amended to match, and a comment on #2
records the measurement — including the withdrawal of the claim that a write counter distinguishes
SSA from `CreateOrUpdate`.**

### 7.2 Existing envtest specs

`internal/controller/wireguard_controller_test.go` is 1929 lines with 22 `It` blocks — 23 specs at
runtime, because the `:1426` block sits inside a two-iteration loop — behind
`//go:build integration`. Any new file must carry that tag or it silently does not run — the
`.golangci.yml` comment block documents this class of failure.

#### Blocking prerequisite: three test sites mangle Service port identity

*Corrected 2026-10-26: an earlier version named one site. There are three, and all three are now
fixed by commit `e7d4579` (plan Task 1).* Each one breaks the `(port, protocol)` merge key of §3.3:

1. **`reconcileServiceWithClusterIP` (`:74`)** replaced the port list with
   `[]corev1.ServicePort{{Port: port, TargetPort: ...}}` — **no `Name`, no `Protocol`**. The
   apiserver defaults `Protocol` to `TCP`, producing a merge key of `(51820, TCP)` distinct from the
   operator's `(51820, UDP)`. The operator's next apply adds a second entry and then fails
   validation permanently with `spec.ports[0].name: Required value`. Seven specs call it: `:911`,
   `:1030`, `:1109`, `:1195`, `:1265`, `:1340`, `:1449`.
2. **The `:322` NodePort spec** called `reconcileServiceWithTypeNodePort(serviceKey,
   expectedNodePort, 5182)` (`:371`). That helper mutates `Ports[0].Port` in place, so the stored
   key became `(5182, UDP)`; the apply then re-adds `(51820, UDP)` named `wireguard`, and validation
   fails with a duplicate port name.
3. **The `:797` DNS search-domain spec** inlined a copy of defect 1 at `:838`.

In every case `Reconcile` requeues and fails identically forever and the `Wireguard` never reaches
ready. Of the two options an earlier version offered, `e7d4579` took the fallback at all three
sites: mutate `Port`/`TargetPort` in place, leaving `Name` and `Protocol` intact (and pass `51820`,
not `5182`). The preferred option, deleting the ClusterIP helper, was not taken.

#### Specs expected to pass unchanged

Corrected against earlier drafts, which wrongly listed five helper-dependent specs here and then
two mangle sites (`:322`, `:797`):

`:189` custom address · `:256` custom DNS · `:476` LoadBalancer e2e ·
`:624` / `:693` address vs externalAddress · `:766` annotations without address ·
`:1487`–`:1578` PersistentKeepalive · `:1580` / `:1699` wstunnel sidecar and dualMode ports ·
`:1812` peer-initialisation race.

#### Specs requiring attention

| spec | why |
|---|---|
| `:322` NodePort e2e, `:797` DNS search-domain override | mangle sites 2 and 3 above; fixed by `e7d4579`, then expected to pass |
| `:855`, `:984`, `:1060`, `:1150`, `:1318` | depend on the helper above; unblocked by fixing it, then expected to pass |
| `:1236` tolerations | depends on the helper; also exercises the deleted scheduling-drift check. Concrete change: none to its assertions. It currently relies on the drift check firing on a *later* reconcile; after the change convergence happens on the next reconcile, so only its `Eventually` timing is at issue. Keep the assertions verbatim; adjust only if it goes red. |
| `:1426` userspace table (2 specs) | **omitted from the previous draft.** Directly exercises the deleted userspace drift check and calls the helper at `:1449`. Same treatment as `:1236`. |
| `:390`, `:570`, `:930` | assert `len(Containers) == 1` and read `Containers[0].Image`. Still correct — the agent is still index 0. **No change required**; the previous draft's instruction to move them to by-name lookup was premised on a §3.2 claim that has been withdrawn. |
| `:447`–`:470` resource-status assertions | verify they still hold once status reads the apply response |

#### Envtest leaks Deployments between specs

`BeforeEach` (`:130`–`:185`) deletes Wireguards, peers, Services, Nodes, Secrets and ConfigMaps —
**not Deployments** — and envtest runs no garbage collector, so owner-reference GC never fires.
`vpn-dep` persists across every spec. This is why the tolerations and userspace specs converge at
all today, and it is a precondition for spec 6 below.

### 7.3 New specs required

The old design's specs encode the old design's surface; passing them all is necessary and not
sufficient.

**Rule: test writes create co-ownership.** Any test that `Update`s an operator-owned object makes the
test client a co-owner of the fields it changed, and a co-owned field is never removed (§6.1). A
test write can therefore mask exactly the removal a spec is trying to prove. Binding on the specs
below, and enforced in the plan's spec code:

- Specs 3, 4 and 6 tamper with an operator-owned object, through `Update` or SSA, **only on a field
  the operator will change back** — a conflict, which the forced apply transfers.
- **No spec `Create`s an operator-owned object except spec 6**, and spec 6 must then exercise a
  removal.

1. **No-churn, per-object write count.** Reconcile an unchanged `Wireguard` several times in a
   dedicated namespace; assert at most one apply per owned object per reconcile and stable
   `resourceVersion`, under the §7.1 constraints.
2. **A field with no bespoke check now converges.** The regression test for the actual bug. Change
   `spec.agent.resources` — which has *no* drift check today — and assert the Deployment updates.
   Fails against `main`, passes after.
3. **Removal converges.** Toggle the wstunnel sidecar off and assert the container count drops;
   remove a `spec.serviceAnnotations` entry and assert it disappears. These are the cases §5.1
   proves a `DeepDerivative` implementation would miss. Drive both through the `Wireguard` spec
   only; a test write to the Deployment or Service in between would co-own the field and mask the
   removal (rule above).
4. **External drift is reverted.** Tamper with the Deployment's agent image using a different field
   manager; assert the next reconcile restores it. The image is a value the operator changes back,
   so this satisfies the rule above.
5. **Server-assigned Service fields survive.** Assert `clusterIP` and an allocated `nodePort` are
   unchanged across repeated reconciles — the guard on §3.3's largest risk.
6. **Adoption of a legacy object — the proof that §6.1 works.** Delete the leaked `vpn-dep` and
   wait for the delete to land (the leak is documented in §7.2). Seed the Deployment as the
   pre-change code would have, with the wstunnel sidecar enabled, via
   `k8sClient.Create(ctx, obj, client.FieldOwner("manager"))`. The explicit owner records exactly
   the production legacy entry — operation `Update`, manager `manager` — where envtest's own
   User-Agent would record `controllers.test`, which production code deliberately does not know
   (§6). Reconcile; then **disable something the operator renders** (the wstunnel sidecar) and
   assert, with `Eventually`, that it is **gone from the live Deployment**. That removal is the
   assertion that proves the fix: a forced apply over a legacy object never conflicts, so "no
   conflict error" alone passes with or without the migration. Also assert that `managedFields`
   no longer carries an `Update` entry for `manager`. Two seeding constraints, reasoned rather than
   tested: seed **before** the `Wireguard` exists, or the operator creates the object by apply first
   and the spec tests nothing; and seed it without owner references, since the `Wireguard` has no
   UID yet (the operator's apply adds them).
7. **Permanent-failure surfacing, and recovery.** Drive the §3.4 ClusterIP-plus-nodePort case and
   assert a Degraded condition appears rather than the operator retrying invisibly. Then remove the
   cause and assert `Degraded` becomes `False` while `Ready` is `True` (§3.4, *Clearing
   `Degraded`*).

---

## 8. Scope

**In scope:** the Deployment path (`:829`–`:921`), the metrics Service (`:489`–`:516`), the main
Service create and drift block (`:553`–`:603`), the status-block reads that depend on them, the
`applyOwned` seam, **the legacy field-manager migration inside it (§6.1)**, the `failDegraded`
error path and the `Degraded` clearing rule (§3.4), and **the fix to all three Service-port mangle
sites in the test file (§7.2; done in `e7d4579`)**, without which the suite cannot pass.

**Also in scope, documentation:** `docs/ROADMAP.md` §Phase 0.2 still carries the stale
`:824`/`:844`/`:861`/`:871` line numbers this design corrects. Fix them in the same change. Likewise
the `.golangci.yml` comment block headed `ENABLED HERE, DELIBERATELY` (`:87`–`:92`) records the
adoption-time complexities `121` / `42` and says #2 "replaces wholesale" the reconcile loop's
`gocyclo` hits, including `updateWireguardPeers`, which this design defers. The current figures are
the 132 and 43 below; correct that comment in the same change.

**Explicitly out of scope:**

- **`updateWireguardPeers` (`:198`, complexity 43).** Deferred by decision. It renders peer
  configuration, not owned Kubernetes objects; #17 and #22 already touch that template. Real debt,
  different concern, and folding it in would make this diff unreviewable.
- **The Secret (`:700`–`:800`).** Already reconciles correctly via `bytes.Equal` on marshalled
  `state.json`. It also carries generated key material and drives the pod restart annotation — a
  materially different risk profile.
- **The ConfigMap (`:804`–`:825`).** Created and never updated, which *looks* like the same defect.
  It is not: `ConfigMapBuilder.ForWireguard` sets only `Labels`, an owner reference, and
  `Data: map[string]string{}`, so it has **no data drift surface**. It exists only as an `envFrom`
  source. Converting it would be motion without value; if it ever gains content it should move to
  `applyOwned` then (#51). The metrics Service is equally spec-independent and is converted anyway
  (§3.3). The ConfigMap has a create-and-return-early branch too (`ConfigMapPending`, `:819`), so
  that half of the reason applies to both; the difference is the other half. Reverting external
  edits is wanted on the metrics Service, but on the ConfigMap it would close the accidental
  user-editable extension point #51 records — a behaviour change that belongs to #51, not here.
- **`spec.metric.resources` (§9).** Rendering it is a separate change.
- **The readiness gates** at `:620` / `:640` / `:670` — they read observed state.
- **Decomposing `Reconcile`.** The lint ratchet (`new-from-merge-base: origin/main`) will begin
  reporting on the lines this work touches, which is the intended effect of paying the debt down,
  not a regression. Complexity falls substantially from its current **132**, but `Reconcile` is not
  being decomposed wholesale here.

---

## 9. Known gap: criterion (b) is not fully met

#2's criterion (b) is *"Changing any pod-affecting spec field triggers an update."* Walking the CRD:

- **Covered today and still covered:** `NodeSelector`, `Tolerations`, `UseWgUserspaceImplementation`,
  `Tunnel.Enabled`.
- **Not covered today, newly covered by this change:** `EnableIpForwardOnPodInit`,
  `Agent.Resources`, `Tunnel.Port`, `Tunnel.Image`, `Tunnel.Resources`. (Init-container removal
  converges on an object the operator created by apply (Q4): the builder's empty
  `[]corev1.Container{}` is `omitempty`-dropped, the operator relinquishes `f:initContainers`, and
  the entry is removed.)
- **Removals on pre-existing objects converge only through §6.1.** Every *removal* in this walk —
  `Tolerations` set to null, `Tunnel.Enabled` or `EnableIpForwardOnPodInit` turned off, a
  `ServiceAnnotations` key deleted, a `Tunnel.DualMode` port dropped — targets a field that, on an
  object written before this change, the legacy manager co-owns wherever the pre-change code
  wrote it. SSA puts such a field back until the migration has run. Reasoned from source; to be
  proven by spec 6.
- **Routed through `state.json`,** which already reconciles: `Mtu`, `Dns`, `DnsSearchDomain`,
  `PeerCIDR*`, `IPv6Only`, `PersistentKeepalive`, `Address`, `ExternalAddress`.
- **Routed through the Service:** `ServiceType`, `NodePort`, `ServiceAnnotations`; `Address`
  also renders into `spec.loadBalancerIP` when the type is LoadBalancer
  (`internal/resources/service.go:90`–`:93`).
- **Routed through both the Service and the peer configs:** `Tunnel.DualMode` (Service ports,
  `service.go:69`; peer configs, `wireguard_controller.go:344`).
- **Read only into status, not pod-affecting:** `Tunnel.Type` (`wireguard_controller.go:980`).
- **Rendered nowhere at all: `spec.metric` (`WireguardPodSpec`, carrying `Resources`).**
  `api/v1alpha1/wireguard_types.go:80` declares it and nothing reads it. It does nothing today and
  will still do nothing after this change.

So criterion (b) is met for every field **except `spec.metric`**. That gap is stated here rather
than glossed; it should be a follow-up issue — either render it or remove the field (#50).

---

## 10. Open questions

### 10.1 Closed 2026-08-30

1. **§7.1 acceptance-criterion correction — CONFIRMED.** #2's third criterion has been amended to
   assert on applies issued per object as well as `resourceVersion` stability, and a comment on #2
   records both measurements with captured output, including the explicit withdrawal of the claim
   that a write counter distinguishes the two strategies.
2. **ConfigMap — §8's reasoning accepted as final for this change.** No drift issue filed, because
   there is no drift surface. The separate finding underneath it — that the ConfigMap and the
   agent's `envFrom` are an inert pair, and that the operator never updating it creates an
   accidental user-editable extension point — is filed as **#51**. Out of scope here.
3. **`spec.metric` — filed as #50, to be deleted rather than rendered.** Out of scope here: #2 is a
   `refactor:` of the reconcile loop, #50 is a `fix:` to the API surface. §9's gap therefore stands
   as a known, tracked limitation of this change rather than an omission.

### 10.2 Open — raised at the design gate, 2026-10-26

The design-gate review could not verify the following. None of them is stated as fact elsewhere
in this document; each stays open until it is measured or traced.

4. **The sixteen Q-results in §4.** The `spike-evidence/` captures are in a session scratchpad, not
   in the repo, and envtest was not re-run at the design gate. §4 stands as recorded at Gate 1.
5. **The VPA claim in §5.2** — that VPA's admission controller mutates Pods and not
   `containers[].resources` on the Deployment template. VPA's documentation could not be fetched
   to confirm it.
6. **SSA ordering of associative-list items when a foreign manager has inserted one.** The
   structured-merge-diff ordering logic (`typed/merge.go` at v4.4.1, the `sharedOrder` walk,
   lines 192–271) was not traced. Relevant to #5's init-container ordering criterion.
7. **Whether the apiserver's `patchAllocatedValues` preserves an allocated `nodePort`** when the
   operator relinquishes a previously owned non-zero `spec.nodePort` (the user sets
   `spec.nodePort` back to 0).
8. **The §7.2 envtest Deployment-leak claim.** Consistent with the `BeforeEach` source; not
   executed.
9. **Behaviour on Kubernetes versions other than 1.30.0** (also §11).

Raised while revising §6.1 rather than by the review, and equally unverified:

10. **Whether a migrated legacy *Service* entry owns server-allocated fields — CLOSED
    2026-10-27, measured: it does not.** The concern was that if the `manager` entry on a
    pre-existing Service owns `spec.clusterIP`, `spec.clusterIPs` or an allocated `nodePort`, the
    migration hands them to the operator and its next apply, which renders none of them,
    relinquishes them. On envtest 1.30.0, after
    `k8sClient.Create(ctx, svc, client.FieldOwner("manager"))` of the builder's NodePort Service,
    the `manager` `Update` entry owns `f:metadata.f:labels`, `f:spec.f:externalTrafficPolicy`,
    `f:spec.f:internalTrafficPolicy`, the 51820/UDP port
    (`k:{"port":51820,"protocol":"UDP"}`: `f:name`, `f:port`, `f:protocol`, `f:targetPort`),
    `f:spec.f:selector`, `f:spec.f:sessionAffinity` and `f:spec.f:type`, but **not** `f:clusterIP`
    or `f:nodePort`. So the migration hands the operator nothing allocated, and adoption kept both
    `clusterIP` and `nodePort` unchanged. The Service variant of spec 6 (plan Task 7 Step 4)
    asserts all three facts. This does not answer item 7, which concerns a `nodePort` the operator
    itself once rendered, and it holds for 1.30.0 only (item 9).
11. **The migration end-to-end.** That the migration patch, itself an `Update` that changes no
    tracked field, leaves no new `Update` entry behind; and that re-defaulting after migration
    (§6.1, *Side effect*) stores an identical spec and triggers no rollout. Reasoned from source;
    to be proven by spec 6, which can also assert `metadata.generation` is unchanged across
    adoption.

---

## 11. Could not verify

- **Behaviour on Kubernetes versions other than 1.30.0.** SSA semantics have been stable since 1.22
  and the storage-layer short circuit is version-independent, but this was not tested across
  versions.
- **Interaction with a real mutating admission webhook** (Kyverno, VPA). §5.2's second argument is
  reasoned from SSA's documented merge semantics, not measured; envtest runs no such webhook.
- **Real `.Status` propagation.** The apply response carries the `.Status` *field*, but envtest runs
  no Deployment controller, so `readyReplicas` was 0 throughout. That the status block reads
  meaningful values in a real cluster is inferred, not observed.
- **Whether the `:1236` tolerations spec passes after the rewrite.** It is blocked upstream by the
  §7.2 helper defect, so its post-change behaviour could not be isolated.
- **What field manager owns objects in any existing production installation.** §6 derives
  `manager` from the image's binary name and the User-Agent mechanism, traced through source, but
  no live cluster was inspected. An installation running a differently named binary is the §6.1
  known limitation.
