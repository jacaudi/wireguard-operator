# Design: Declarative Deployment and Service reconciliation

- **Issue:** [#2](https://github.com/jacaudi/wireguard-operator/issues/2) — `[P0] Replace trigger-based Deployment drift checks with declarative reconciliation`
- **Status:** Approved 2026-08-30 (Gate 1 findings folded in; §10 closed)
- **Date:** 2026-08-29
- **Base commit:** `be6d4e4`
- **Depends on:** #1 (closed by PR #39, which consolidated rendering onto `internal/resources`)
- **Unblocks:** #5 (`spec.initContainers` / `spec.extraContainers`), #6 (`spec.volumes`), #7 (reserved-name validation)

> **Anchoring note.** Line numbers below are correct at `be6d4e4` and are a *secondary* hint only.
> Issue #2's own line numbers went stale across PRs #39 and #49; this document will too. Every
> in-scope block is therefore also identified by its function or by a distinctive string, and an
> implementation plan derived from this must locate blocks by those strings, not by line number.

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
that owns the field-manager identity and the GVK requirement.

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

// applyOwned renders an operator-owned object into the cluster with Server-Side Apply.
//
// What the operator ends up owning is what `json.Marshal` of the built object emits, which
// is NOT the same as "the fields the builder sets" in either direction. Zero-valued structs
// are emitted despite `omitempty` (so the operator owns `spec.strategy`, the agent
// container's `resources`, and the pod template's `creationTimestamp`, none of which the
// builder sets); zero-valued scalars with `omitempty` are dropped (so a `nodePort` of 0 is
// NOT owned, which is what lets the server keep its allocation — see §3.3).
//
// SSA requires apiVersion/kind in the request body and a typed object's TypeMeta is empty
// after construction, so the GVK is derived from the scheme and stamped on before the call.
func (r *WireguardReconciler) applyOwned(ctx context.Context, obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, r.Scheme)
	if err != nil {
		return err
	}
	obj.GetObjectKind().SetGroupVersionKind(gvk)
	return r.Patch(ctx, obj, client.Apply,
		client.FieldOwner(FieldManager), client.ForceOwnership)
}
```

Verified (§4, Q13): `apiutil.GVKForObject` resolves `apps/v1, Kind=Deployment` from the scheme, so
no GVK is hardcoded; and the apply **response is decoded back into `obj`**, returning with `uid`,
`resourceVersion`, `.Status` and `managedFields` populated, at a `resourceVersion` identical to a
subsequent live `Get`.

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
because the apply response already carries `.Status`. The standalone `Get` is deleted.

The create-and-return-early branch disappears: apply is create-or-update in one request, so there
is no create path to special-case and no synthetic requeue.

**API-call accounting, stated correctly.** The deleted `Get` cost *zero* apiserver round-trips —
the manager's client is cache-backed by default. In today's steady state none of the four drift
checks fire, so a no-change reconcile issues *zero* writes. After this change every reconcile
issues **three** unconditional SSA PATCHes (Deployment, main Service, metrics Service). The net is
**+3 apiserver round-trips per reconcile, not −1.** That is an acceptable trade at this operator's
scale — a handful of `Wireguard` objects — and it buys convergence on every field. It is recorded
here because an earlier draft of this document claimed the opposite.

### 3.3 Service path

Two Services are involved; both move to `applyOwned`:

- **metrics Service** (`:489`–`:516`) — created once, never reconciled. Becomes an apply.
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

This is not hypothetical: it is exactly what the existing test helper produces (§7.2, B1). The
production analogue — a foreign writer setting a different protocol on the WireGuard port — is
narrow but real, and is why §3.4 exists.

### 3.4 Error path must surface, not spin

Two reachable conditions produce a *permanent* apply failure rather than a transient one: the port
collision above, and `spec.nodePort` set together with `serviceType: ClusterIP` (the builder sets
`NodePort: wg.Spec.NodePort` unconditionally at `internal/resources/service.go:53`, and the
apiserver rejects `nodePort` on a ClusterIP Service). The second is pre-existing — today the
`Create` fails identically — but in both cases a bare `log.Error` + `return ctrl.Result{}, err`
leaves the operator retrying forever with a stale `Status.Status` and nothing user-visible.

Every apply error therefore routes through a small helper that records a Degraded condition before
returning:

```go
func (r *WireguardReconciler) failDegraded(
	ctx context.Context, req ctrl.Request, err error, reason, msg string,
) (ctrl.Result, error) {
	ctrllog.FromContext(ctx).Error(err, msg)
	if serr := r.updateStatus(ctx, req, metav1.Condition{
		Type: ConditionDegraded, Status: metav1.ConditionTrue,
		Reason: reason, Message: err.Error(),
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
logged instead: the caller needs the original error unwrapped, since wrapping would defeat the
`errors.IsConflict`-style checks used elsewhere in this file, and the status write is retried on
the next reconcile anyway.

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
| Q15 | `spec.replicas` vs a foreign writer | Forced back to 1 — pre-existing behaviour, not a regression. | ✅ measured at Gate 1 |
| Q16 | LB ingress status survives the apply | **Yes**, and is present in the apply response. | ✅ measured at Gate 1 |

Additionally verified from source at Gate 1: the no-op-write short circuit lives in
`k8s.io/apiserver/pkg/storage/etcd3/store.go` inside `GuaranteedUpdate`
(`if !origState.stale && bytes.Equal(data, origState.data)`) — not Deployment-specific, so §5.2's
durability claim holds; and the TypeMeta requirement is enforced server-side in
`apimachinery/pkg/util/managedfields/internal/versioncheck.go`, so it is robust to client-library
version skew.

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
2. **It overwrites foreign field managers on every reconcile** (Q8). The scenario that matters is a
   cluster policy engine or VPA mutating the Deployment, not a service mesh — mesh sidecars are
   injected at the Pod level and never appear in the Deployment. One qualification: the operator
   owns the agent container's `resources` as an *empty* field, which does not conflict with a VPA
   owning `resources.requests.cpu` — so VPA survives *while `spec.agent.resources` is unset*. Once
   a user sets it, force-apply stomps the VPA every reconcile. That is intended convergence, but
   the advantage is conditional, not absolute.

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
operation `Update`, named from the client's User-Agent.

- **A conflict is raised only when the applied value *differs* from a value owned by another
  manager.** Q3: the first apply after a `Create` of identical content succeeds without force.
- **`client.ForceOwnership` is nonetheless required** for the first apply that changes a
  previously `Update`-owned field; without it that apply returns a conflict and the operator
  wedges. Verified at Gate 1: the un-forced conflict message is
  `Apply failed with 1 conflict: conflict with "<manager>" using apps/v1: .spec.template.spec.containers[name="agent"].image`.
  Hence the helper passes force unconditionally rather than opportunistically.
- **Adoption costs exactly one extra write per object, once**, then stabilises.
- **Force-apply is also what makes convergence work** — it takes ownership back from a human who
  ran `kubectl edit`, which is the behaviour #2 wants.
- **Stale owner references rotate correctly.** `ownerReferences` is `+listType=map` keyed on `uid`;
  when a `Wireguard` is deleted and recreated with a new UID, the apply relinquishes the old entry
  and adds the new one with no conflict (verified at Gate 1). This matters because envtest leaks
  Deployments across specs (§7.2).

**No RBAC change is required.** `config/rbac/role.yaml` already grants `patch` on `services`
(line 19, core-group rule) and on `deployments` (line 39, `apps` rule), matching the kubebuilder
markers at `wireguard_controller.go:414` and `:416`. `patch` is the verb SSA uses.

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
current code can produce — and persist zero of them (`resourceVersion` stable).

There is a fair counter-reading: at this scale a wasted round-trip is cheap, and `resourceVersion`
is what actually drives watch events and rollouts. Both assertions are kept. **This strengthening
was confirmed on 2026-08-30: #2's third criterion has been amended to match, and a comment on #2
records the measurement — including the withdrawal of the claim that a write counter distinguishes
SSA from `CreateOrUpdate`.**

### 7.2 Existing envtest specs

`internal/controller/wireguard_controller_test.go` is 1929 lines / ~27 specs, behind
`//go:build integration`. Any new file must carry that tag or it silently does not run — the
`.golangci.yml` comment block documents this class of failure.

#### Blocking prerequisite: the ClusterIP test helper wedges the apply

`reconcileServiceWithClusterIP` (`:74`) replaces the port list with
`[]corev1.ServicePort{{Port: port, TargetPort: ...}}` — **no `Name`, no `Protocol`**. The apiserver
defaults `Protocol` to `TCP`, producing a merge-key of `(51820, TCP)` distinct from the operator's
`(51820, UDP)`. Per §3.3 the operator's next apply adds a second entry and then fails validation
permanently with `spec.ports[0].name: Required value`. `Reconcile` requeues and fails identically
forever; the `Wireguard` never reaches ready.

Seven specs call it: `:911`, `:1030`, `:1109`, `:1195`, `:1265`, `:1340`, `:1449`.

**This must be fixed as part of this change**, and it is in scope (§8). Two options; the plan must
pick one:

- **Preferred — delete the helper and its call sites.** Its only apparent purpose is to satisfy the
  ClusterIP readiness gate, which checks `len(svcFound.Spec.Ports) > 0` only. The operator's own
  apply already guarantees that, so the mutation is vestigial.
- **Fallback — preserve identity.** Have the helper mutate `Port`/`TargetPort` in place, leaving
  `Name` and `Protocol` intact.

#### Specs expected to pass unchanged

Corrected against the previous draft, which wrongly listed five helper-dependent specs here:

`:189` custom address · `:256` custom DNS · `:322` NodePort e2e · `:476` LoadBalancer e2e ·
`:624` / `:693` address vs externalAddress · `:766` annotations without address ·
`:797` DNS search-domain override · `:1487`–`:1578` PersistentKeepalive ·
`:1580` / `:1699` wstunnel sidecar and dualMode ports · `:1811` peer-initialisation race.

#### Specs requiring attention

| spec | why |
|---|---|
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

1. **No-churn, per-object write count.** Reconcile an unchanged `Wireguard` several times in a
   dedicated namespace; assert at most one apply per owned object per reconcile and stable
   `resourceVersion`, under the §7.1 constraints.
2. **A field with no bespoke check now converges.** The regression test for the actual bug. Change
   `spec.agent.resources` — which has *no* drift check today — and assert the Deployment updates.
   Fails against `main`, passes after.
3. **Removal converges.** Toggle the wstunnel sidecar off and assert the container count drops;
   remove a `spec.serviceAnnotations` entry and assert it disappears. These are the cases §5.1
   proves a `DeepDerivative` implementation would miss.
4. **External drift is reverted.** Tamper with the Deployment's agent image using a different field
   manager; assert the next reconcile restores it.
5. **Server-assigned Service fields survive.** Assert `clusterIP` and an allocated `nodePort` are
   unchanged across repeated reconciles — the guard on §3.3's largest risk.
6. **Adoption.** Delete the leaked `vpn-dep` and wait for the delete to land, seed a Deployment via
   `Create` (the pre-change shape), then reconcile; assert convergence with no conflict error.
   Covers the upgrade path for existing installations. The explicit delete-and-wait is required
   because of the leak documented in §7.2.
7. **Permanent-failure surfacing.** Drive the §3.4 ClusterIP-plus-nodePort case and assert a
   Degraded condition appears rather than the operator spinning silently.

---

## 8. Scope

**In scope:** the Deployment path (`:829`–`:921`), the metrics Service (`:489`–`:516`), the main
Service create and drift block (`:553`–`:603`), the status-block reads that depend on them, the
`applyOwned` seam, the `failDegraded` error path (§3.4), and **the `reconcileServiceWithClusterIP`
test-helper fix (§7.2)**, without which the suite cannot pass.

**Also in scope, documentation:** `docs/ROADMAP.md` §Phase 0.2 still carries the stale
`:824`/`:844`/`:861`/`:871` line numbers this design corrects. Fix them in the same change.

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
  `applyOwned` then (#51).
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
  converges: the builder's empty `[]corev1.Container{}` is `omitempty`-dropped, the operator
  relinquishes `f:initContainers`, and the entry is removed.)
- **Routed through `state.json`,** which already reconciles: `Mtu`, `Dns`, `DnsSearchDomain`,
  `PeerCIDR*`, `IPv6Only`, `PersistentKeepalive`, `Address`, `ExternalAddress`.
- **Routed through the Service:** `ServiceType`, `NodePort`, `ServiceAnnotations`.
- **Rendered nowhere at all: `spec.metric` (`WireguardPodSpec`, carrying `Resources`).**
  `api/v1alpha1/wireguard_types.go:80` declares it and nothing reads it. It does nothing today and
  will still do nothing after this change.

So criterion (b) is met for every field **except `spec.metric`**. That gap is stated here rather
than glossed; it should be a follow-up issue — either render it or remove the field (#50).

---

## 10. Open questions — all closed 2026-08-30

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
- **What field manager owns objects in any existing production installation.** §6 reasons from
  User-Agent derivation, confirmed as the mechanism, but no live cluster was inspected.
