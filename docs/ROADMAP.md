# Roadmap: WireGuard gateway operator

This document scopes the work to turn this operator into a Kubernetes-native VPN
gateway: a WireGuard server that terminates client tunnels and forwards their
traffic onward through [gluetun](https://github.com/qdm12/gluetun) to a commercial
VPN provider, with clients that can be declared explicitly or injected into
existing workloads automatically.

Each numbered item below is tracked as a GitHub issue; [#37](https://github.com/jacaudi/wireguard-operator/issues/37) is the umbrella.

---

## Target architecture

```
                                    ┌─────────────────────────────────┐
  in-cluster pod                    │  wireguard server pod           │
  ┌────────────────────┐            │                                 │
  │ app container      │            │  ┌───────────┐   ┌───────────┐  │
  │ wg client sidecar  │──ClusterIP─┼─▶│ wg agent  │──▶│  gluetun  │──┼──▶ provider
  └────────────────────┘            │  └───────────┘   └───────────┘  │
                                    │  ┌───────────┐   ┌───────────┐  │
  phone / laptop ──LoadBalancer─────┼─▶│ wstunnel  │   │ port-sync │  │
                                    │  └───────────┘   └───────────┘  │
  desktop (obfuscated) ──Gateway────┘                                 │
                                    └─────────────────────────────────┘
```

The server pod is a single network namespace shared by the agent, gluetun, and any
user-supplied sidecars — which is what makes gluetun's `tun0` and its default route
usable by the WireGuard agent without any CNI involvement.

---

## Decisions

Recorded with rationale so they don't get relitigated.

### Single server, not HA

**Decision:** one server replica. No sharding, no multi-peer client configs, no
failover sidecar.

**Rationale:** a generic WireGuard client already reconnects on its own after the
server pod reschedules, with no client-side logic, provided (a) the endpoint address
is stable and (b) the server key persists. Both already hold — a ClusterIP or
LoadBalancer VIP survives rescheduling, and `secretForWireguard` persists the key.
WireGuard has no connection to lose; a client with traffic to send retries the
handshake every ~5s.

Recovery budget: ~20–40s for a pod crash (dominated by gluetun re-establishing its
upstream), and ~5min for node failure unless `tolerationSeconds` is tuned down —
which `spec.tolerations` already allows today.

HA would have bought elimination of that window at the cost of multi-peer config
generation, a failover sidecar with an API watch, metric semantics, per-server
credential management, and a permanent split between how internal and external
clients fail over. Not worth it for the failure budget.

**Forward compatibility:** don't hardcode "exactly one server" into peer config
generation or IPAM, so HA remains additive later.

### Three client paths

| client | path | config flavor |
|---|---|---|
| in-cluster pod | ClusterIP, direct UDP | internal |
| phone / stock client | LoadBalancer VIP, direct UDP | direct |
| desktop on a hostile network | Gateway `HTTPRoute` → wstunnel | tunnel |

Mobile clients **cannot** use the wstunnel config: `PreUp`/`PostUp`/`PostDown`/
`FwMark`/`Table` are wg-quick features and are
[explicitly unsupported](https://git.zx2c4.com/wireguard-apple/about/MOBILECONFIG.md)
by the official WireGuard Apple apps (Android likewise). They need plain UDP.

`spec.tunnel.dualMode` already emits multiple config flavors per peer, so this is
mostly config generation plus a new internal variant.

### LoadBalancer by default for raw UDP; UDPRoute opt-in

Envoy Gateway supports [UDPRoute](https://gateway.envoyproxy.io/docs/tasks/traffic/udp-routing/)
as a first-class feature, so the Gateway path is viable. Two caveats drive the
default:

- Envoy proxies UDP **non-transparently**, so the server records Envoy's per-session
  socket as the peer endpoint. That socket lives in Envoy's UDP session table with an
  idle timeout, which makes `PersistentKeepalive` load-bearing rather than optional.
- Every VPN packet traverses a userspace proxy instead of staying in the CNI datapath.
  For a bulk-throughput gateway that is the hot path.

So: LoadBalancer Service by default, `UDPRoute` behind a flag for users who want
everything expressed as Gateway API. Gateway API is used unconditionally for the
wstunnel path, where L7 termination is the actual value.

Note for docs: Cilium's *Gateway API implementation* cannot mix L4 and L7 routes on
one Gateway. This does not apply when using Envoy Gateway with Cilium as CNI only.

### Implicit clients are minted per Pod, not per Deployment

A single peer identity shared across replicas breaks: WireGuard tracks one endpoint
per public key, learned from the most recent authenticated packet. Three replicas
sharing a key means the server delivers pod A's return traffic to whichever pod
handshook last. It misdelivers silently and oscillates under load.

Mutating Pods instead gives each pod its own peer, tunnel IP, and table entry — correct
at any replica count, and it covers StatefulSets, DaemonSets and bare Pods for free.
Opt-in labels still live on the Deployment's pod template.

### Opt-in is a label, not an annotation

Webhook `objectSelector` matches labels only. With a label, the API server invokes the
webhook only for pods that opted in; with an annotation, the webhook sits in the
admission path for every pod in the cluster.

### Killswitch is enforced in two layers

In-pod rules (work on any CNI) plus an operator-generated CNI policy (survives sidecar
crash or `wg0` teardown — the failure that actually causes leaks).

---

## Leak analysis

Where traffic can escape the tunnel, since several issues below exist to prevent it.

**wg-quick's full-tunnel setup already fails closed** for `AllowedIPs` changes. It
installs policy routing (`ip rule ... table 51820` plus `suppress_prefixlength 0`), so
internet-bound packets cannot use the main table's default route, and WireGuard drops
packets matching no peer. Reassigning `AllowedIPs` is therefore safe on its own.

Real leak sources:

| side | cause | mitigation |
|---|---|---|
| client | `wg0` or the sidecar dies — policy rules vanish with it | native sidecar (`restartPolicy: Always`), CNI policy |
| client | app containers start before the tunnel is up | native sidecar ordering + startup probe |
| client | config covers `0.0.0.0/0` but not `::/0` | deny v6 egress in both layers |
| server | gluetun's `tun0` gone; `-o eth0` MASQUERADE or node-level SNAT picks traffic up | bind egress to the tunnel interface + terminal DROP |
| server | `iptables-restore` without `--noflush` wipes gluetun's killswitch on every peer change | operator-owned chains + `--noflush` |

Inbound connections arriving through the tunnel on a forwarded port are decapsulated
inside the pod netns and never cross the veth as plaintext, so CNI policy neither sees
nor blocks them. The killswitch does not interfere with port forwarding.

---

## Phase 0 — Foundations

Blocks everything else.

### 0.1 Consolidate the two divergent Deployment builders ([#1](https://github.com/jacaudi/wireguard-operator/issues/1))

Two paths render the Deployment and the newer one is dead code.
`internal/resources/deployment.go:54` (`DeploymentBuilder.ForWireguard`) is constructed
at `internal/controller/wireguard_controller.go:1000` but never called; the live path is
`deploymentForWireguard` at `:1110`, called from `:810`, `:825`, `:846`, `:863`, `:874`.
They have already drifted — the unused one adds a `metrics` port and uses `HTTPPort`.

Every feature below touches Deployment rendering. Implementing in `internal/resources`
alone would have no runtime effect.

- [ ] Exactly one function renders the Deployment
- [ ] Drift resolved intentionally, not by arbitrarily picking a copy
- [ ] Existing controller tests pass unchanged

### 0.2 Replace trigger-based Deployment drift checks with declarative reconciliation ([#2](https://github.com/jacaudi/wireguard-operator/issues/2))

The Deployment is only rebuilt when one of four specific checks fires: agent image
(`:824`), userspace flag (`:844`), tunnel sidecar (`:861`), scheduling settings (`:871`).
Any new spec field would need its own bespoke check or changes would silently not apply.

- [ ] Desired Deployment computed once and semantically diffed against actual
- [ ] Changing any spec field that affects the pod triggers an update
- [ ] No hot-loop update churn from server-defaulted fields

### 0.3 Rework iptables generation: non-flushing, operator-owned chains, correct egress ([#3](https://github.com/jacaudi/wireguard-operator/issues/3))

`ApplyRules` runs `iptables-restore` **without `--noflush`**
(`internal/iptables/iptables.go:14-21`), replacing the whole `*nat` and `*filter` tables
in the pod's network namespace on every peer sync — which will wipe gluetun's killswitch
on every peer change. Separately, the NAT rule hardcodes `-o eth0`
(`:124` and `:155` for v6), which would bypass the VPN entirely once gluetun is in play.

- [ ] Rules applied with `--noflush` into chains the operator owns
- [ ] gluetun's rules survive a peer sync
- [ ] Egress interface is derived, not hardcoded
- [ ] Terminal DROP so traffic not leaving via the tunnel is dropped rather than falling
      through to node-level SNAT
- [ ] Equivalent v4 and v6 handling

### 0.4 Add `PersistentKeepalive` to generated peer configs ([#4](https://github.com/jacaudi/wireguard-operator/issues/4))

The config template (`internal/controller/wireguard_controller.go:322-370`) emits
`PrivateKey`, `Address`, `DNS`, optional `MTU`, and the `[Peer]` block — no keepalive.
An idle client won't retry until it has traffic of its own, and its NAT/conntrack entry
expires so the server cannot initiate toward it. This becomes load-bearing on the Envoy
UDPRoute path, where the recorded endpoint is a proxy session with an idle timeout.

- [ ] Keepalive emitted in all config flavors, interval configurable with a sane default

---

## Phase 1 — Pod extension points

### 1.1 Add `spec.initContainers` and `spec.extraContainers` ([#5](https://github.com/jacaudi/wireguard-operator/issues/5))

Today the only init container is the hardcoded `sysctl` one gated on
`spec.enableIpForwardOnPodInit`, and the only extra sidecar is `wstunnel`. There is no
user-facing extension point.

Use native sidecars (`restartPolicy: Always`) for long-running containers. Prefer raw
`[]corev1.Container` over a narrowed struct — constraining it means users hit a wall on
some field we didn't anticipate.

- [ ] Both fields render into the pod
- [ ] Ordering relative to the built-in `sysctl` init container is defined and documented
- [ ] CRD schema size increase is acceptable

### 1.2 Add `spec.volumes` and volume mounts for user-supplied containers ([#6](https://github.com/jacaudi/wireguard-operator/issues/6))

Sidecars need to share data — a port-forward sync service has to read the port gluetun
writes to a file. Container fields alone are not enough.

- [ ] Extra volumes render into the pod spec
- [ ] User containers can mount both their own volumes and the built-in ones

### 1.3 Validate user-supplied containers ([#7](https://github.com/jacaudi/wireguard-operator/issues/7))

- [ ] Reject name collisions with `agent`, `wstunnel`, `sysctl`, `gluetun`
- [ ] Clear validation errors surfaced as a status condition, not a silent drop

---

## Phase 2 — gluetun

Modeled as a typed `spec.gluetun` block rendered through the generic sidecar machinery
from Phase 1 — one rendering path, but the operator can compute what a passthrough
cannot (firewall ports, readiness gating, forwarded-port status).

### 2.1 Design the `spec.gluetun` API ([#8](https://github.com/jacaudi/wireguard-operator/issues/8))

Provider selection, credentials by `secretRef` (never inline), server/region filters,
port-forwarding toggle, image and resources.

- [ ] Credentials referenced, never stored in the Wireguard object
- [ ] Schema documented with at least one worked example

### 2.2 Render the gluetun sidecar ([#9](https://github.com/jacaudi/wireguard-operator/issues/9))

Requires `/dev/net/tun` and `NET_ADMIN`; the agent container currently runs
`readOnlyRootFilesystem` with `NET_ADMIN` and no privilege escalation, so the two
security contexts need reconciling.

### 2.3 Provider credentials via `secretRef` ([#10](https://github.com/jacaudi/wireguard-operator/issues/10))

- [ ] Credential rotation does not require a CRD edit
- [ ] Credentials never appear in the Wireguard object or its status

### 2.4 Auto-compute gluetun firewall input ports and outbound subnets ([#11](https://github.com/jacaudi/wireguard-operator/issues/11))

gluetun's killswitch will block the inbound WireGuard port and the kubelet health probes
unless configured. Probe replies routed out the tunnel mean the pod CrashLoops.

- [ ] `FIREWALL_INPUT_PORTS` derived from the WireGuard, health and tunnel ports
- [ ] `FIREWALL_OUTBOUND_SUBNETS` covers the pod and node CIDRs
- [ ] Health probes succeed with the killswitch active

### 2.5 Route peer egress through the gluetun tunnel ([#12](https://github.com/jacaudi/wireguard-operator/issues/12))

Depends on 0.3.

- [ ] Peer traffic masquerades out the tunnel interface
- [ ] Verified by observing the exit IP, not just by rule inspection

### 2.6 Reconcile DNS between gluetun's resolver and `spec.dns` ([#13](https://github.com/jacaudi/wireguard-operator/issues/13))

gluetun runs a DNS-over-TLS listener on `127.0.0.1:53`, which interacts with the `dns`
field handed to peers.

- [ ] Documented precedence, no silent override of an explicit `spec.dns`

### 2.7 Gate Wireguard readiness on gluetun tunnel health ([#14](https://github.com/jacaudi/wireguard-operator/issues/14))

Without this, the Service routes to a server whose upstream is down — traffic either
blackholes or leaks. This is also what makes rescheduling safe.

- [ ] Readiness reflects tunnel state, not just process liveness
- [ ] Status condition distinguishes "tunnel down" from "pod starting"

### 2.8 Surface gluetun's forwarded port in Wireguard status ([#15](https://github.com/jacaudi/wireguard-operator/issues/15))

Prerequisite for any port-sync sidecar.

- [ ] Forwarded port and exit IP in status, updated when they change

### 2.9 E2E: egress exits via the VPN and fails closed when the tunnel drops ([#16](https://github.com/jacaudi/wireguard-operator/issues/16))

- [ ] Test asserts the observed public IP is the provider's, not the node's
- [ ] Test kills the tunnel and asserts traffic stops rather than leaking

---

## Phase 3 — Client paths and ingress

### 3.1 Emit an internal peer config flavor pointing at the ClusterIP ([#17](https://github.com/jacaudi/wireguard-operator/issues/17))

Today even the "direct" config targets `serverAddress` + `Status.Port`, i.e. the
*external* address (`wireguard_controller.go:363-369`). In-cluster clients would hairpin
out to the LoadBalancer or Gateway and back in.

A ClusterIP is also the only endpoint safe against WireGuard's resolve-once behaviour —
per-pod DNS resolves to a pod IP that changes on reschedule, and WireGuard never
re-resolves.

- [ ] Injected clients get an endpoint of `<svc>.<ns>.svc.cluster.local:<port>`

### 3.2 LoadBalancer UDP path for mobile and stock clients ([#18](https://github.com/jacaudi/wireguard-operator/issues/18))

`spec.serviceType: LoadBalancer` and `spec.address` already exist; this is mostly
making it a documented first-class path with a stable VIP.

- [ ] Stock client config verified importable and working on iOS and Android

### 3.3 Render an `HTTPRoute` for the wstunnel path ([#19](https://github.com/jacaudi/wireguard-operator/issues/19))

The client-side tunnel config is already generated
(`wireguard_controller.go:335-344`); what's missing is the route object.

- [ ] `HTTPRoute` rendered with a `parentRef` to a configured Gateway
- [ ] WebSocket upgrade proxied through to the wstunnel container
- [ ] `ReferenceGrant` only where genuinely required — a route and Service in the same
      namespace need none; Gateway attachment is governed by the Gateway's
      `allowedRoutes.namespaces`

### 3.4 Optional `UDPRoute` rendering for the direct path ([#20](https://github.com/jacaudi/wireguard-operator/issues/20))

Behind a flag, with the datapath tradeoff documented.

### 3.5 Derive peer endpoints from `spec.gateway.hostname`, watch Gateway status ([#21](https://github.com/jacaudi/wireguard-operator/issues/21))

Endpoint derivation currently walks `spec.externalAddress` → LoadBalancer ingress →
NodePort plus node addresses (`wireguard_controller.go:595-670`). Add the Gateway as a
source. Prefer a declared hostname as the single source of truth for both the route and
the peer configs; watch Gateway status for readiness reporting only.

### 3.6 Per-path MTU for direct vs tunnel configs ([#22](https://github.com/jacaudi/wireguard-operator/issues/22))

The tunnel path carries TLS and WebSocket framing over TCP and wants a lower MTU.
`spec.mtu` is currently a single value.

---

## Phase 4 — Webhook infrastructure

No webhook exists today: `cmd/manager/main.go:99` starts a server on 9443 but nothing
registers a handler, there is no `config/webhook`, and `PROJECT` has no webhook entries.

### 4.1 Webhook scaffolding ([#23](https://github.com/jacaudi/wireguard-operator/issues/23))

Server wiring, `config/webhook` manifests, `PROJECT` entries, RBAC.

### 4.2 Certificate management and failure-policy scoping ([#24](https://github.com/jacaudi/wireguard-operator/issues/24))

cert-manager or self-signed rotation, plus `failurePolicy` and `objectSelector` scoping.
A webhook in the pod admission path that fails open-ended can block pod creation
cluster-wide.

- [ ] Webhook only invoked for pods carrying the opt-in label
- [ ] Operator outage cannot block unrelated pod creation

---

## Phase 5 — Implicit clients

### 5.1 Define the label-based opt-in contract ([#25](https://github.com/jacaudi/wireguard-operator/issues/25))

Labels (not annotations) so `objectSelector` can scope the webhook. Label values are
limited to alphanumerics, `-`, `_`, `.`, max 63 chars — fine for a server name reference.

- [ ] Opt-in label and server-reference label defined and documented
- [ ] Cross-namespace server references expressible

### 5.2 Pod mutating webhook: inject the WireGuard client sidecar ([#26](https://github.com/jacaudi/wireguard-operator/issues/26))

The webhook mutates only. It must not create the peer — webhooks with side effects break
`--dry-run` and get retried unpredictably.

- [ ] Injects sidecar, volumes and config mount
- [ ] Respects `dryRun`
- [ ] Idempotent on pod update

### 5.3 Pod controller: mint and reap a WireguardPeer per pod ([#27](https://github.com/jacaudi/wireguard-operator/issues/27))

The config Secret cannot exist when the webhook runs, and doesn't need to — kubelet
holds the pod in `ContainerCreating` until the referenced Secret appears.

**Owner references cannot cross namespaces.** If the peer lives in the server's namespace
and the pod in the workload's, an ownerRef is invalid and the GC will *delete the peer*.
Cleanup must be an explicit controller reaping orphans.

- [ ] Peer minted per pod with deterministic naming
- [ ] Peers reaped when their pod goes away, with no cross-namespace ownerRef
- [ ] Rapid create/destroy does not leak peers or addresses

### 5.4 Split the aggregated peer-config Secret into per-peer Secrets ([#28](https://github.com/jacaudi/wireguard-operator/issues/28))

All peer configs currently land in one `<wg-name>-peer-configs` Secret keyed by peer name
(`wireguard_controller.go:229`), containing **every peer's private key**. Mounting that
into an app pod would hand it every other peer's credentials.

- [ ] Each peer's config available in its own Secret
- [ ] Injected pods mount only their own
- [ ] Migration path for the existing aggregated Secret

### 5.5 Cross-namespace `wireguardRef` with bidirectional consent ([#29](https://github.com/jacaudi/wireguard-operator/issues/29))

`wireguardpeer_controller.go:167` resolves `spec.wireguardRef` in the peer's own
namespace. Add an optional namespace, and gate attachment on the server opting in
(`spec.allowedPeerNamespaces`) — otherwise any namespace can attach itself to the VPN
and egress through a paid tunnel.

- [ ] Attachment requires both the peer naming the server and the server permitting the
      namespace
- [ ] Rejected attachments surface a clear status condition

### 5.6 Make peer IPAM instance-scoped rather than namespace-scoped ([#30](https://github.com/jacaudi/wireguard-operator/issues/30))

`checkDuplicateAddress` lists peers with `client.InNamespace(namespace)`
(`wireguardpeer_controller.go:241`), and `GetUsedIPs` builds the used set from that list.
The allocator itself is stateless and cannot leak addresses
(`internal/ipam/allocator.go:43`) — but shown a namespace-filtered list while peers exist
elsewhere, it will hand out **duplicate tunnel IPs**.

- [ ] Used-address set scoped by `wireguardRef` across all namespaces
- [ ] Regression test covering peers for one server split across namespaces

### 5.7 Replicate a peer's config Secret into the workload's namespace ([#31](https://github.com/jacaudi/wireguard-operator/issues/31))

Secrets cannot be mounted across namespaces.

- [ ] Replica kept in sync and removed with the peer
- [ ] Only the owning pod's namespace receives it

### 5.8 Client sidecar: kernel and userspace paths as a native sidecar ([#32](https://github.com/jacaudi/wireguard-operator/issues/32))

An init container that exits cannot hold a userspace tunnel — `wireguard-go` is a
process, and the interface dies with it. Since userspace fallback is a headline feature,
the client must be a native sidecar (`restartPolicy: Always`), which also gives ordering
ahead of app containers and clean teardown.

- [ ] Works with the kernel module and with userspace fallback
- [ ] App containers do not start before the tunnel is up

### 5.9 In-pod egress lockdown for injected clients ([#33](https://github.com/jacaudi/wireguard-operator/issues/33))

- [ ] Non-tunnel egress dropped inside the pod
- [ ] WireGuard's own fwmark-tagged output excepted, or the rules strangle the tunnel
- [ ] IPv6 denied explicitly
- [ ] Cluster DNS and a configurable in-cluster allow list still reachable

### 5.10 Operator-generated CNI network policy for injected clients ([#34](https://github.com/jacaudi/wireguard-operator/issues/34))

The layer that survives sidecar crash or `wg0` teardown, because it is enforced outside
the pod. In Cilium, an endpoint with any egress rule is default-deny for egress, so the
allow list *is* the killswitch — and adding `toEntities: world` anywhere silently
destroys it.

Suggested shape: one clusterwide baseline policy matching the opt-in label (server plus
DNS only), with per-workload policies adding narrow allows.

- [ ] Policy generated per injected workload, opt-out available
- [ ] Falls back to a plain `NetworkPolicy` off Cilium
- [ ] Test asserts egress is dropped when the tunnel is down

---

## Phase 6 — Project

### 6.1 Docs and examples ([#35](https://github.com/jacaudi/wireguard-operator/issues/35))

Worked examples for each of the three client paths, gluetun setup, and the implicit
client flow.

### 6.2 Fork housekeeping ([#36](https://github.com/jacaudi/wireguard-operator/issues/36))

The Go module path is still `github.com/nccloud/wireguard-operator`.
