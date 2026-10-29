# Design: minimal in-cluster VPN gateway

**Status:** revision 2 after the design gate, 2026-10-29. Base: `origin/main` `929e0fe`.
**Supersedes:** `docs/ROADMAP.md` (the 37-issue programme). Facts this design depends on are inlined; nothing here requires that file or the abandoned iptables draft.
**gluetun pin:** `ghcr.io/qdm12/gluetun:v3.41.3@sha256:fa19cc76b2af13d57a8d3dc3066f2ada061b1c761b8aecf989b3877c0486e027`. Source line references are to that tag unless marked *master*.

---

## 1. Goal and scope

The owner's words: *"take the wireguard operator that works, update it to set the client inside a pod and the traffic goes to a server that has its traffic egressed to a VPN thanks to Gluetun. That's it. External clients … can/will come later. If we need to … blow everything else in this repo away please do it."*

| In scope (MVP) | Out of scope (must stay *possible*, must not be *built*) |
|---|---|
| A WireGuard client sidecar in an application pod | Phones, laptops, any client outside the cluster |
| One WireGuard server pod per `Wireguard`, in the same namespace as its clients | LoadBalancer / NodePort Services, `externalAddress`, wstunnel, Gateway API |
| The server pod egresses through a gluetun sidecar to a commercial provider | Cross-namespace attachment, webhook injection, CNI NetworkPolicy generation |
| Fail-closed on both ends by **routing**; in-cluster traffic and cluster DNS keep working | IPv6, per-peer egress policy, bandwidth limits, metrics, HA, DNS through the gateway |

---

## 2. Verdict: trim to the skeleton, do not re-scaffold

**Trim.** Keep the module, API group, CRD kinds, the SSA plumbing, the agent's netlink/wgctrl code, IPAM, the Dockerfiles and the CI pipeline. Delete every feature that exists for external clients. About 60 % of the non-test Go goes; the surviving 40 % is the part that was hard to get right.

- The reusable pieces are the hard-won ones: `applyOwned` with the legacy field-manager migration (`internal/controller/apply.go:62-94`), link/peer sync through wgctrl (`internal/wireguard/wireguard.go:152-212`, `:351-471`), the stateless allocator (`internal/ipam/allocator.go:44`), the aligned CI (`.github/workflows/ci.yaml`). A re-scaffold recreates boilerplate and throws those away.
- The API group stays `vpn.wireguard-operator.io`: `PROJECT` carries `domain: wireguard-operator.io`, `group: vpn`, and renaming changes the CRDs' fully-qualified names for nothing.
- Breaking `v1alpha1` in place is acceptable: the fork has published no release and no chart (README "How to deploy" defers to #48), so there are no stored objects to migrate.

### 2.1 Kept

| Area | What stays | Where |
|---|---|---|
| Controller | `applyOwned`, `failDegraded`, condition handling, server key Secret, `state.json`, the pod-annotation nudge that makes kubelet refresh the Secret volume | `apply.go:62-119`; `wireguard_controller.go:121-171`, `:654-754`, `:687-709` |
| Builders | `DeploymentBuilder` skeleton and `agentContainer`, ClusterIP `Service`, `SecretBuilder.ForWireguard`, `LabelsForWireguard` | `resources/deployment.go:54-212`; `service.go:47-101`; `secret.go:37-56`; `builder.go:27-32` |
| Agent | fsnotify state watcher, `SyncLink` with kernel→userspace fallback, peer add/remove/update via wgctrl, `/health` | `agent/agent.go:62-160`; `wireguard/wireguard.go:152-237`, `:351-494`; `cmd/agent/main.go:113-141` |
| IPAM | `AllocateIP`, `GetUsedIPs`, `DefaultPeerCIDR4` | `ipam/allocator.go:31`, `:44`, `:63` |
| Images, CI | `images/agent/Dockerfile` (debian:trixie, iptables, wireguard-tools), `images/manager/Dockerfile`, release-please, lint and integration stages | `images/`, `.github/` |
| Tests | envtest skeleton, `apply_test.go`, `churn_test.go`, the kind harness | `controller/suite_test.go`, `internal/it/suite_test.go` |

### 2.2 Deleted

| Feature | Files / fields / code | Why |
|---|---|---|
| wstunnel | `TunnelSpec` (`wireguard_types.go:81-105`), `Status.Tunnel` (`:124`), `wstunnelContainer` (`deployment.go:214-253`), tunnel Service port (`service.go:58-74`), tunnel configs (`wireguard_controller.go:339-372`, `:564-572`, `:847-855`), `DefaultWstunnelImage`, `DefaultTunnelPort` | External-client obfuscation |
| LoadBalancer / NodePort / external address | `Address`, `ExternalAddress`, `ServiceType`, `NodePort`, `ServiceAnnotations` (`wireguard_types.go:35-61`); `getNodeIps` (`:92-119`); LB/NodePort branches (`:574-622`); `LoadBalancerIP` (`service.go:90-94`); `examples/serverWithNodePortService.yaml` | The Service is always ClusterIP |
| Peer config rendering | `updateWireguardPeers` (`:210-419`) and the `<name>-peer-configs` Secret; `PrivateKey` (`wireguardpeer_types.go:24-26`, `:51`); `SecretBuilder.ForPeer` (`secret.go:59-77`); key minting (`wireguardpeer_controller.go:106-153`) | Private keys are minted in the client pod and never stored (§ 3.4) |
| Per-peer egress policy, speed, DNS, AllowedIPs, Disabled | `wireguardpeer_types.go:42-49`, `:58-96`, `:115-120`; `iptables.go:72-113`, `:178-224` | No consumer; every peer is forwarded the same way |
| **The whole server iptables path** | `Iptables.Sync` and `GenerateIptableRulesFromPeers` (`iptables.go:14-176`), its call at `cmd/agent/main.go:97-100` | `ApplyRules` runs `iptables-restore` **without** `--noflush` (`:14-21`) and masquerades `-o eth0` (`:124`). Left in place past M1 it would wipe gluetun's rules and leak. Deleted in M1, replaced in M4 |
| IPv6 | `PeerCIDRv6`, `IPv6Only` (`wireguard_types.go:45-51`); `AddressV6` (`wireguardpeer_types.go:41`); `GetUsedIPv6IPs`; `GenerateIp6tableRulesFromPeers`; v6 branches (`wireguard.go:286-306`) | The v6 path never worked: `::PREROUTING` chain headers, IPv4 DNS/server addresses reused, IPv4-only `--reject-with`, chain names containing `:`, no test. Clients deny v6 outright (§ 3.6) |
| Server-pushed DNS | `Dns`, `DnsSearchDomain` on both specs; `Status.Dns`; kube-dns lookup (`:519-542`); `IsStateValid` checks on `Status.Address` and `Status.Dns` (`agent.go:33-41`) | Clients keep cluster DNS. `IsStateValid` keeps only the private-key and per-peer checks |
| Metrics | `internal/agent/wireguard_metrics.go`; `ForWireguardMetrics` (`service.go:104-130`); `MetricsPort`; `spec.metric` (`:80`, #50); `config/prometheus/`; the prometheus module; e2e "exposes Prometheus metrics" (`internal/it/it_test.go`) | No present consumer |
| Vestigial ConfigMap | `configmap.go`; `:756-779`; `envFrom` (`deployment.go:177-183`) (#51) | Empty `Data` |
| `Status.Resources`, `Status.Address/Port/UniqueIdentifier` | `wireguard_types.go:113-148`; `:793-890` | Replaced by `status.endpoint`, `status.publicKey`, conditions |
| Knobs with one value | `UseWgUserspaceImplementation` (`:65`; the fallback at `wireguard.go:189-199` is automatic), `EnableIpForwardOnPodInit` (`:63`; forwarding is unconditional), `PersistentKeepalive` (`:66-74`; constant 25), `Mtu` (`:34`; constant 1320) | KISS |
| Second controller | `WireguardPeerReconciler` (whole file) | One controller (§ 3.2) |
| OLM | `bundle/`, `config/manifests/`, `config/scorecard/`, the `bundle*` Makefile targets | Never published |
| Dead listener | `WebhookServer` option (`cmd/manager/main.go:99`) | Nothing registers a handler |

### 2.3 New

| Piece | Where described |
|---|---|
| `spec.localCIDRs`, `spec.gluetun` on `Wireguard`; `status.address` on `WireguardPeer` | § 3.2 |
| `agent client` subcommand in the existing agent image; `iproute2` added to that image | § 3.3, § 3.6 |
| Server datapath: blackhole table, two `ip rule`s, five iptables rules, 10 s re-assert | § 3.5 |
| `ClusterRole wireguard-operator-client` | § 3.4 |
| e2e: fake exit node via gluetun's `custom` provider, two canaries, a leak counter | § 4 |

### 2.4 Constants (one definition each, in `internal/resources`)

| Constant | Value | Used by |
|---|---|---|
| `WireguardPort` | 51820 | Service, agent listen port, `FIREWALL_INPUT_PORTS` |
| `HTTPPort` | 8080 | agent `/health`, `FIREWALL_INPUT_PORTS` |
| `EgressInterface` | `tun0` | gluetun `VPN_INTERFACE`, agent rules |
| `WgInterface` | `wg0` | server and client link name |
| `TunnelMTU` | 1320 | gluetun `WIREGUARD_MTU`, client `wg0` MTU (= gluetun's own default, `settings/wireguard.go:197`) |
| `Keepalive` | 25 s | client peer config |
| `PeerTable`, `PeerRulePref`, `BlackholeTable`, `BlackholePref` | 51821, 90, 51822, 102 | § 3.5 |
| `ReassertInterval` | 10 s | § 3.5 |
| `GluetunHealthURL` | `http://127.0.0.1:9999/` | agent `/health` |
| `GluetunImage` | the pinned digest above | Deployment |

---

## 3. Architecture

```
 application pod (ns: media)                 gateway pod (ns: media)              provider
 ┌──────────────────────────────┐            ┌─────────────────────────────────┐
 │ app        ──default route──▶│  wg/UDP    │ agent: wg0 10.8.0.1/24          │
 │ wg-client sidecar: wg0       │──ClusterIP─▶ eth0:51820 ─▶ FORWARD ─▶ tun0 ─┼──▶ VPN server
 │  10.8.0.7/32, AllowedIPs 0/0 │            │ gluetun: tun0, policy routing   │
 │ localCIDRs ──▶ eth0 (CNI)    │            │ iif wg0 → table 51822 blackhole │
 └──────────────────────────────┘            └─────────────────────────────────┘
          │ kube-dns, other pods, Services: never enter the tunnel
```

### 3.1 Prior art: angelnu/pod-gateway

pod-gateway solves this problem with VXLAN plus a mutating webhook ([README](https://github.com/angelnu/pod-gateway)). Copied here:

- **Fail-closed by routing on the client.** `client_init.sh` reads the CNI gateway from the default route, adds host routes for `NOT_ROUTED_TO_GATEWAY_CIDRS` via it, then deletes the v4 and v6 default routes and asserts `ping 8.8.8.8` fails ([client_init.sh:16-30](https://github.com/angelnu/pod-gateway/blob/main/bin/client_init.sh)). `NOT_ROUTED_TO_GATEWAY_CIDRS` defaults to `""` (`config/settings.sh:12`); the `10.0.0.0/8 192.168.0.0/16` default belongs to the *gateway-side* `VPN_LOCAL_CIDRS` (`:30`).
- **Fail-closed by routing on the gateway** too: `ip rule add from all lookup main suppress_prefixlength 0 preference 50` ([gateway_init.sh:46-49](https://github.com/angelnu/pod-gateway/blob/main/bin/gateway_init.sh)) keeps main-table *specifics* ahead of the VPN while denying the main default route. Prior art for § 3.5.
- **Gateway and VPN share one netns**; killswitch = `FORWARD` policy `DROP` plus `ACCEPT -o $VPN_INTERFACE` (`gateway_init.sh`).
- The most-starred deployment runs gluetun with its firewall off and relies on pod-gateway's rules ([carpenike/k8s-gitops](https://github.com/carpenike/k8s-gitops/blob/main/kubernetes/cluster-0/apps/vpn/pod-gateway/media/helmrelease.yaml): `FIREWALL: "off"`, `VPN_INTERFACE: wg0`). That variable name was only read up to v3.38; v3.41.3 reads `FIREWALL_ENABLED_DISABLING_IT_SHOOTS_YOU_IN_YOUR_FOOT` (`settings/firewall.go:131`), so that config has silently run with gluetun's firewall *on* since.

Where it pays and WireGuard changes the trade:

| pod-gateway | Cost | WireGuard here |
|---|---|---|
| VXLAN is unauthenticated; any pod knowing `VXLAN_ID` joins | Identity is a shared number | A peer *is* a public key |
| DHCP via dnsmasq (`gateway_sidecar.sh`) | Two daemons, 12 h leases | The operator allocates `/32`s; cryptokey routing replaces DHCP |
| FDB points at the gateway *pod IP*; `client_sidecar.sh:19-30` pings every 10 s and re-runs init on failure | Tight loop, no backoff | Endpoint is the ClusterIP; WireGuard re-handshakes on its own |
| Webhook needed because init runs with `NET_ADMIN` before the app | Certs, failure policy | Same capability; a user-added native sidecar is enough (§ 3.3) |
| Clients' DNS goes to the gateway's dnsmasq | Resolver per gateway | MVP keeps cluster DNS; deferred (§ 8) |

Where WireGuard is worse: per-client key lifecycle (§ 3.4), and two nested tunnels make MTU a first-class concern (constant 1320 everywhere).

### 3.2 CRDs: two, both trimmed

```go
type WireguardSpec struct {
    PeerCIDR   string      `json:"peerCIDR,omitempty"` // default 10.8.0.0/24 (ipam.DefaultPeerCIDR4)
    // Pod, Service and node CIDRs (and 169.254.20.10/32 if NodeLocal DNSCache is in use). Clients route
    // these via the CNI, never via the tunnel; the gateway hands them to gluetun as FIREWALL_OUTBOUND_SUBNETS.
    // +kubebuilder:validation:MinItems=1
    LocalCIDRs []string    `json:"localCIDRs"`
    Gluetun    GluetunSpec `json:"gluetun"`
    NodeSelector map[string]string   `json:"nodeSelector,omitempty"`
    Tolerations  []corev1.Toleration `json:"tolerations,omitempty"`
    Agent        WireguardPodSpec    `json:"agent,omitempty"`
}
type GluetunSpec struct {
    Env       []corev1.EnvVar             `json:"env,omitempty"`     // passthrough
    EnvFrom   []corev1.EnvFromSource      `json:"envFrom,omitempty"` // provider credentials: a Secret, never inline
    Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}
```

Status on `Wireguard`: `conditions` (`Ready`, `Progressing`, `Degraded`), `status.endpoint` (`<clusterIP>:51820`), `status.publicKey`.

Validation, surfaced as `Degraded`: the Service ClusterIP falls inside `localCIDRs`; `peerCIDR` overlaps no `localCIDRs` entry; no reserved gluetun variable in `env`/`envFrom` keys (§ 3.5). Documented, not validated: `localCIDRs` must not overlap the provider's tunnel range ([firewall.md](https://github.com/qdm12/gluetun-wiki/blob/main/setup/options/firewall.md) says the same of `FIREWALL_OUTBOUND_SUBNETS`).

Passthrough rather than a typed provider schema: gluetun has dozens of providers and the community configures it exactly this way (kubesearch: 58 HelmReleases set `FIREWALL_OUTBOUND_SUBNETS` in `env`). A typed schema would be a second copy of gluetun's contract.

**`WireguardPeer`** keeps `spec.wireguardRef` (same namespace, as `wireguardpeer_controller.go:156` already resolves it) and `spec.publicKey`. The allocated address moves from `spec.address` (today written by the controller at `wireguard_controller.go:268-276`) to **`status.address`**: the sidecar has no `/status` RBAC, so it cannot choose its own address. Nothing else is copied into the peer; the sidecar reads the server's endpoint, public key and `localCIDRs` from the `Wireguard` object itself.

**Allocation and uniqueness.** Allocation runs inside one `Wireguard` reconcile over one list, appending to `usedIps` as it goes (`:277`), so two unallocated peers in the same pass cannot collide. Across passes the informer cache can lag a status write, so the controller (1) writes `status.address` with `Status().Update`, which fails with 409 on a stale object, and (2) before rendering `state.json`, dedupes: if two peers carry the same address, the one with the oldest `creationTimestamp` keeps it and the others have `status.address` cleared and are re-allocated on the next pass. A duplicate never reaches `wg0`. The e2e scales a Deployment to five replicas to exercise this (§ 4).

**One controller.** `WireguardReconciler` gains `Watches(&WireguardPeer{}, EnqueueRequestsFromMapFunc(peer → peer.Spec.WireguardRef))` and drops `Owns(&WireguardPeer{})` (`:918`), because the peer's controller owner is the Pod. The peer controller's other duties (Pending status `:96-104`, duplicate check `:187-195`, server ownerRef `:200-214`) are absorbed or unnecessary.

**Ready semantics.** Pod readiness: the agent's readiness probe (`/health`) is true when `wg0` is configured *and* gluetun's health server answers 200 (§ 3.5). CR condition `Ready=True` when the Deployment reports `readyReplicas == 1` and every apply succeeded; `Progressing` while not; `Degraded` on apply or validation failure. "Tunnel down" therefore shows as `Ready=False`, reason `DeploymentNotReady`, at `kubectl get wireguard`. Lands in M4.

### 3.3 How a pod gets its client: a user-added native sidecar

The user adds one init container with `restartPolicy: Always` (native sidecar; GA in Kubernetes 1.33, on by default since 1.29 — [kubernetes.io](https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/)). It must be the **first** init container, so later init containers also run inside the tunnel.

```yaml
spec:
  serviceAccountName: media-vpn            # RoleBinding to ClusterRole wireguard-operator-client
  volumes:
  - { name: wg-key, emptyDir: { medium: Memory } }
  initContainers:
  - name: wg-client
    image: ghcr.io/jacaudi/wireguard-operator/agent:<tag>
    restartPolicy: Always
    args: ["client", "--wireguard", "egress"]
    env:
    - { name: POD_NAME,      valueFrom: { fieldRef: { fieldPath: metadata.name } } }
    - { name: POD_NAMESPACE, valueFrom: { fieldRef: { fieldPath: metadata.namespace } } }
    - { name: POD_UID,       valueFrom: { fieldRef: { fieldPath: metadata.uid } } }
    - { name: POD_IP,        valueFrom: { fieldRef: { fieldPath: status.podIP } } }
    - { name: HOST_IP,       valueFrom: { fieldRef: { fieldPath: status.hostIP } } }
    securityContext: { capabilities: { add: [NET_ADMIN] } }
    volumeMounts: [{ name: wg-key, mountPath: /var/run/wireguard }]
    startupProbe: { exec: { command: ["agent", "client", "--check"] }, periodSeconds: 2, failureThreshold: 90 }
```

No liveness probe. The sidecar is a configurer; with kernel WireGuard the datapath is netns state that runs without it, and a hung process is not a leak. The kubelet restarts it on exit. (A handshake-age liveness check would restart-loop: WireGuard rekeys on the first send after 120 s and rejects a session at 180 s, so handshake age legitimately cycles up to ~145 s with a 25 s keepalive.)

`--check` is the startup probe only: `wg0` exists with the expected peer, its last handshake is younger than 180 s, and `ip route get 1.1.1.1` resolves to `dev wg0`. That holds app containers until the tunnel is up and the routes are in place.

Prerequisites, documented in M6: the namespace must run at PodSecurity level `privileged` (the client needs `NET_ADMIN`, which `baseline` forbids; the gateway's sysctl init is privileged). The pod's ServiceAccount token is visible to the app containers; accepted, because the token's only power is the two verb sets in § 3.4 within the namespace. Users who want less can set `automountServiceAccountToken: false` and project a token into the sidecar alone; nothing in the operator changes.

Alternatives rejected:

| Option | Why not |
|---|---|
| Mutating webhook (pod-gateway's way) | It cannot solve the hard part. At admission a pod may have only `generateName`, so no per-pod key or Secret can exist yet; the sidecar still has to obtain identity at runtime. The webhook only saves pasting the block above, at the price of certs, `failurePolicy` and a cluster-wide admission path (#23, #24, #26). Deferred; it would inject exactly this block |
| Label + pod controller that mints a Secret per pod | A Secret volume cannot be named from the downward API, so the sidecar still fetches by API and the SA needs `get secrets` namespace-wide. Broader than § 3.4 |
| One `WireguardPeer` per Deployment | WireGuard keeps one endpoint per key, learned from the last authenticated packet. Replicas sharing a key get each other's return traffic, silently and oscillating under load |

### 3.4 Identity and addresses: one key per pod, minted in the pod

`agent client` (stdlib `flag`, a `FlagSet` per subcommand; the bare `agent` invocation keeps today's server flags at `cmd/agent/main.go:28-34`):

1. Refuse to start if `POD_IP == HOST_IP` (hostNetwork): deleting that default route would take the node offline.
2. Load the private key from `/var/run/wireguard/key` or generate and write it. Memory-backed, so a container restart reuses it and it never touches etcd or disk.
3. Get `WireguardPeer/<POD_NAME>`. If it exists with a controller ownerRef whose `uid != POD_UID` (a StatefulSet pod reusing the name), delete it with a UID precondition. Create `{wireguardRef, publicKey, ownerReferences: [{apiVersion: v1, kind: Pod, name: POD_NAME, uid: POD_UID, controller: true}]}`. Same namespace, so GC reaps the peer with the pod. No reaper controller.
4. Watch the peer and the `Wireguard`. On the peer: wait for `status.address`; recreate on a DELETE event. On the `Wireguard`: read `status.endpoint`, `status.publicKey`, `spec.localCIDRs`; re-run steps 5–6 on every change, so a `localCIDRs` edit is applied live without a pod restart.
5. `wg0` via the existing `SyncLink` (`wireguard.go:152-212`); `syncAddress` must *replace* rather than return early when an address exists (`:71-73`); one peer, `AllowedIPs 0.0.0.0/0`, `Endpoint = status.endpoint`, `PersistentKeepalive = 25`, MTU 1320. Server-side `Sync` reuse is partial: `ListenPort`, gateway-address derivation (`:323-349`) and the `MTU` const 1420 (`:23`) are server-only.
6. Routes (§ 3.6).

RBAC shipped in `config/rbac/`: `ClusterRole wireguard-operator-client` = `wireguardpeers: create get list watch delete` + `wireguards: get list watch`. Admins bind it per namespace; namespace RBAC is the authorization boundary for who may attach.

Controller side, per peer with a `publicKey`: allocate `status.address` if empty, dedupe (§ 3.2), include in `state.json` (`:661-665`). The server agent already adds, updates and removes peers by public key (`wireguard.go:372-462`) with `AllowedIPs` fixed to the `/32` (`:398`, `:452`).

### 3.5 The server pod: agent and gluetun in one netns

Containers: privileged `sysctl` init (`net.ipv4.ip_forward=1`, today's `deployment.go:256-268`, now unconditional), `agent`, `gluetun`. Deployment strategy `Recreate`, one replica.

**gluetun container.** `securityContext.capabilities.add: [NET_ADMIN]`, no read-only root (it writes `/gluetun` and `/tmp`), no probes of its own (the agent polls it), and `lifecycle.postStart: (ip rule del table 51820; ip -6 rule del table 51820) || true`, which gluetun's Kubernetes page prescribes ([kubernetes.md](https://github.com/qdm12/gluetun-wiki/blob/main/setup/advanced/kubernetes.md)): after an abrupt container exit the stale rule makes `RuleAdd` fail with "file exists" (`wireguard/rule.go:19-24`) on every restart. No `/dev/net/tun` mount: gluetun creates it itself (`internal/tun/tun.go`; present in the tree, not read).

**Reserved variables**, rendered by the operator and rejected (`Degraded/ReservedGluetunEnv`) if present in `spec.gluetun.env` or in a referenced Secret's keys:

| Variable | Value | Why |
|---|---|---|
| `VPN_TYPE` | `wireguard` | The routing below assumes gluetun's WireGuard-mode tables and marks |
| `VPN_INTERFACE` | `tun0` | The image `ENV` sets `tun0` (`Dockerfile:79`), but the WireGuard settings default is `wg0` when the variable is unset (`settings/wireguard.go:196`), and kernelspace mode deletes an existing WireGuard link of its name (*master* `wireguard/run.go`, `setupKernelSpace`). A user clearing the variable would destroy the agent's `wg0` |
| `FIREWALL_ENABLED_DISABLING_IT_SHOOTS_YOU_IN_YOUR_FOOT` | `on` | gluetun's own killswitch stays; the only name v3.39+ reads (`settings/firewall.go:131`) |
| `FIREWALL_INPUT_PORTS` | `51820,8080` | `INPUT` policy is `DROP` (`firewall/iptables.go:95-104`); this emits `INPUT -i eth0 -p tcp/udp --dport` accepts (`:204-205`) |
| `FIREWALL_OUTBOUND_SUBNETS` | `join(localCIDRs, ",")` | Emits `OUTPUT -o eth0 -s <podIP> -d <cidr> ACCEPT` (`:163-169`) and a route via the CNI gateway in table 199 behind `ip rule pref 99` (*master* `routing/outbound.go`), carrying the agent's encapsulated UDP back to client pods |
| `HEALTH_SERVER_ADDRESS` | `127.0.0.1:9999` | The agent's `/health` polls it (image default, `Dockerfile:165`) |
| `WIREGUARD_MTU` | `1320` | Pins the tunnel MTU the client constant matches; *master* changed the default to 0/PMTUD |

Everything else is passthrough. Replies to kubelet probes already work: gluetun adds `from <podIP>/32 lookup 200 pref 100` with a default route via eth0 in table 200 (`routing/inbound.go:11-12`, `:63-75`).

**Fail-closed by routing (the safety mechanism).** Lookup order for a packet arriving on `wg0` bound for the internet:

```
pref  90   to 10.8.0.0/24            lookup 51821   # ours: table 51821 = 10.8.0.0/24 dev wg0 (return path)
pref  98-100                                        # gluetun: local nets, outbound subnets, inbound-from-podIP
pref 101   not fwmark 51820          lookup 51820   # gluetun: default via tun0 — only while tun0 is up
pref 102   iif wg0                   lookup 51822   # ours: table 51822 = blackhole default
          main                                      # never reached for iif wg0
```

With `tun0` up, 101 forwards into the tunnel. With `tun0` gone, with gluetun crashed, or with gluetun's firewall rolled back, table 51820 is empty or the rule is absent, and 102 blackholes everything that came in on `wg0`. The main-table default via eth0 is unreachable for peer traffic no matter what iptables says. Rule 90 is why provider replies to `10.8.0.x` reach `wg0` instead of re-entering `tun0` (gluetun installs `not fwmark <mark> lookup <mark>` with its `AllowedIPs` routes in that table, *master* `wireguard/rule.go`, `route.go`); the `suppress_prefixlength` rule in pod-gateway's `gateway_init.sh:46-49` serves the same purpose.

**iptables (the availability mechanism).** Needed because gluetun sets `FORWARD` policy `DROP` and never adds `FORWARD` accepts (`enable()` has none), and because egress must be masqueraded:

```
iptables -P FORWARD DROP                                                     # asserted by us too
iptables -A FORWARD -i wg0 -o tun0 -s 10.8.0.0/24 -j ACCEPT
iptables -A FORWARD -i tun0 -o wg0 -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
iptables -t mangle -A FORWARD -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu
iptables -t nat -A POSTROUTING -s 10.8.0.0/24 -o tun0 -j MASQUERADE
```

Five iptables lines, two `ip rule`s, two one-route tables. Applied with `-C || -A` on every state change and every 10 s. The loop exists for recovery, not safety: gluetun's `disable()` runs `--flush` and `--delete-chain` and sets all policies `ACCEPT` (`firewall/enable.go:44-60`, `iptables.go:88-93`), and `fallbackToDisabled` calls it whenever `enable()` fails part-way (`enable.go:63-70`), including on every CrashLoop iteration. During that window the blackhole rule drops peer traffic; ten seconds later the accepts and masquerade are back. The MSS clamp lives in `mangle` because a `filter` rule after an `ACCEPT` is never reached.

**Readiness.** The agent's `/health` (`cmd/agent/main.go:113-141`) additionally does `GET http://127.0.0.1:9999/`; gluetun answers 200 with an empty body only when its checks pass and 500 with the error text otherwise (`healthcheck/handler.go`). Loopback is shared across the pod. An unhealthy tunnel makes the server `NotReady`, the Service drops its only endpoint, and clients stop handshaking.

**Considered: gluetun firewall off.** It removes the co-tenancy problem but discards gluetun's protection of its *own* traffic, and whether gluetun's routing component still installs the inbound and outbound-subnet rules with the firewall off is unverified. With safety now carried by routing, the firewall question is about availability only; revisit if the re-assert loop misbehaves.

### 3.6 The client side

Routing, following pod-gateway's order with `wg0` for `vxlan0`:

1. `GW` = gateway of the current default route; if the default route is already `dev wg0` (re-entry after a restart), `GW` = gateway of any existing `localCIDRs` route on `eth0`. Abort if neither exists.
2. For each `localCIDRs` entry: `RouteReplace <cidr> via GW dev eth0`. Remove routes for entries no longer listed.
3. `RouteReplace default dev wg0` (atomic; no window without a default). Delete the v6 default route.

- **What stays local:** everything in `localCIDRs`: pod CIDR, Service CIDR, node CIDR. That covers kube-dns, other pods, Services, the gateway's ClusterIP (validated to be inside `localCIDRs`, so no separate `/32` route is needed), and kubelet probe replies to the node. `resolv.conf` is untouched; in-cluster names resolve as before. Nothing in-cluster enters the tunnel, which is why the server needs no `wg0 → eth0` rule.
- **What goes through the tunnel:** every other destination, via `default dev wg0`, `AllowedIPs 0.0.0.0/0`.
- **Fail-closed:** without an eth0 default route, a vanished `wg0` leaves no route to the internet: `ENETUNREACH`. Kernel WireGuard: `wg0` is netns state and survives a sidecar crash, so traffic keeps flowing until the kubelet restarts the sidecar and it re-syncs idempotently. Userspace fallback: the TUN device dies with the process and the missing default route fails closed. IPv6 fails closed because its default route is gone; dual-stack in-cluster v6 breaks, documented.
- **No iptables on the client.** `NET_ADMIN` is for netlink only.

---

## 4. MVP acceptance criteria and the e2e test

1. A client pod's request to a destination outside `localCIDRs` is answered, and the answer proves it travelled through the gateway's `tun0`.
2. With the tunnel down, the same request gets no answer, and a counter proves no peer traffic left the gateway on `eth0`.
3. Throughout, `nslookup kubernetes.default` and a request to a neighbour pod succeed from the client.
4. Deleting the client pod deletes its `WireguardPeer` (GC) and the server removes the peer from `wg0`.
5. Deleting the gateway pod: the client recovers without a restart once the new pod is Ready.
6. A Deployment scaled to five replicas gets five distinct `status.address` values.

**Without provider credentials.** gluetun's `custom` provider accepts a plain WireGuard server: `VPN_SERVICE_PROVIDER=custom`, `WIREGUARD_ENDPOINT_IP` (must be an IP), `WIREGUARD_ENDPOINT_PORT`, `WIREGUARD_PUBLIC_KEY`, `WIREGUARD_PRIVATE_KEY`, `WIREGUARD_ADDRESSES` ([custom.md](https://github.com/qdm12/gluetun-wiki/blob/main/setup/providers/custom.md)). The test runs **real gluetun** against a **fake exit node** in kind:

```
client pod ──wg──▶ gateway (agent+gluetun) ──wg (custom provider)──▶ exit pod   lo: 203.0.113.10  → "exit"
kind node (hostNetwork canary)                                                 lo: 203.0.113.10  → "node"
```

- **Exit pod:** two containers. The agent image (which gains `iproute2` in M1; trixie's `wireguard-tools` does not pull it in) runs a shell script: `ip link add wg0 type wireguard`, `wg setconf` with gluetun as the only peer, `MASQUERADE -o eth0`, `ip addr add 203.0.113.10/32 dev lo`. `hashicorp/http-echo -listen=:80 -text=exit` answers on every local address. A ClusterIP Service gives gluetun a stable `WIREGUARD_ENDPOINT_IP`.
- **gluetun health without internet:** `HEALTH_TARGET_ADDRESSES=203.0.113.10:80` and `HEALTH_ICMP_TARGET_IPS=203.0.113.10`. The checker uses TLS only when the address ends in `:443` (`healthcheck/checker.go:223`), so port 80 is a plain TCP dial. Env names verified at `settings/health.go:134-136`.
- **Node canary:** a `hostNetwork` pod adds `203.0.113.10/32` to the node's `lo` and answers `node`. Belt and braces: leaked gateway traffic would carry src `10.8.0.x`, so the canary's *reply* would not return, but its access log and the counter below still show the leak.
- **Leak counter:** the test adds `iptables -t mangle -A POSTROUTING -o eth0 -s 10.8.0.0/24` (no target) inside the agent container and asserts its packet count stays zero across every scenario.
- **Scenarios:** (a) tunnel up: `curl http://203.0.113.10/` returns `exit`; (b) exit pod scaled to zero, wait for the gateway to go `NotReady` (budget 3 min; gluetun's checks run on a 1 min / 5 min cadence, `checker.go:66-79`): `curl` fails, counter zero; (c) `kill 1` in the gluetun container, which drives `fallbackToDisabled` paths on restart: `curl` fails or returns `exit`, never `node`, counter zero; (d) delete the gateway pod: client recovers (criterion 5); (e) criteria 3, 4 and 6.
- Harness: the kind suite (`internal/it/suite_test.go`, `make run-e2e` at `Makefile:169-175`) with the wait fixed to `status.observedGeneration == metadata.generation` (#45).

---

## 5. Milestones

Each is one PR, mergeable on its own; main never carries the old flushing agent next to gluetun.

| # | Goal | Issue |
|---|---|---|
| M1 | Trim: every row in § 2.2 including `Iptables.Sync`, `IsStateValid`'s Address/DNS checks, the bundle Makefile targets and `internal/it/it_test.go`'s metrics spec; add `iproute2` to the agent image; regenerate CRDs/RBAC; a `Wireguard` still yields a ClusterIP server and `publicKey` peers still get addresses (temporarily in `spec.address`) | new |
| M2 | One controller: fold the peer controller into `WireguardReconciler`, `status.address` with dedupe, `Wireguard.status.{endpoint,publicKey}`, the § 3.2 validations; envtest specs | new |
| M3 | e2e in CI: fix the wait, wire `make run-e2e` into `ci` as a stage with the surviving "server starts" spec | #44, #45 |
| M4 | Gateway: `spec.gluetun`, `spec.localCIDRs`, reserved variables, gluetun container, unconditional sysctl init, `Recreate`, agent datapath (tables, rules, re-assert, `/health` polling gluetun), Ready semantics; e2e: fake exit node, gateway becomes Ready, rules and routes present, counter zero while a hand-configured test peer sends traffic | #3, #11, #16 |
| M5 | Client: `agent client`, ClusterRole, the § 3.3 block as `examples/client.yaml`; e2e scenarios (a)–(e) — **the MVP** | #32 |
| M6 | README, PodSecurity and `localCIDRs` prerequisites, a worked `media` namespace | #35 |

M4's "hand-configured test peer" is a test pod using `wg` and the agent image with a `WireguardPeer` the test creates; it exists only so M4 can prove the datapath before M5 exists.

---

## 6. Issue triage

| Issue | Action | Note |
|---|---|---|
| #3 iptables rework | **re-scope → M4** | Routing-based fail-closed, five static rules; no owned chains |
| #5, #6, #7 initContainers / extraContainers / volumes / validation | **close** | No consumer; gluetun is typed |
| #8 design `spec.gluetun` | **close** | Decided here |
| #9, #10, #12, #14 render gluetun, secretRef, route egress, readiness on tunnel | **close as superseded by M4** | `envFrom.secretRef`, reserved variables, `/health` polling |
| #11 firewall ports and subnets | **re-scope → M4** | Reserved-variable rendering and rejection |
| #13 DNS | **re-scope, deferred** | "Route client DNS through the gateway"; `spec.dns` is deleted |
| #15 forwarded port / exit IP | **survive, deprioritise** | Needs control-server auth config |
| #16 e2e egress + fail-closed | **re-scope → M4/M5** | § 4 |
| #17 internal config flavor | **close** | One flavor; `status.endpoint` is the ClusterIP |
| #18, #19, #20, #21, #22 LB, HTTPRoute, UDPRoute, Gateway hostname, per-path MTU | **close** | External clients |
| #23, #24, #26 webhook | **survive, deprioritise** | Would inject the § 3.3 block |
| #25 label contract | **close** | No label without a webhook |
| #27 pod controller mint/reap | **close** | Self-registration + Pod ownerRef |
| #28, #31 aggregated Secret, replication | **close** | No Secret, no private keys in etcd |
| #29 cross-namespace | **survive** | Post-MVP |
| #30 IPAM instance-scoped | **close** | `getWireguardPeers` filters by `wireguardRef` (`:84`) and feeds `GetUsedIPs` (`:229`); the stale-cache case is handled by dedupe (§ 3.2) |
| #32 client sidecar | **re-scope → M5** | |
| #33 in-pod lockdown | **close as superseded by M5** | Routing-based |
| #34 CNI policy layer | **survive, deprioritise** | |
| #35 docs | **re-scope → M6** | One path |
| #37 epic | **re-scope** | Body = § 5 |
| #44, #45 e2e tier, flaky wait | **re-scope → M3** | |
| #46, #48 release-republish, Helm chart | **survive** | |
| #50, #51 `spec.metric`, ConfigMap | **close as superseded by M1** | |
| #52, #53 forwarded port, cycle VPN | **survive, deprioritise** | |
| #55, #57, #58, #59, #60, #61, #62, #63, #64 apply.go / envtest housekeeping | **survive** | `apply.go` kept as-is |
| #56 gates made unreachable by SSA | **close as superseded by M1/M2** | |
| #65 clusterIP/nodePort preservation spec | **re-scope** | clusterIP only |

Counts over the 52 open issues: **close 24** (#5 #6 #7 #8 #9 #10 #12 #14 #17 #18 #19 #20 #21 #22 #25 #27 #28 #30 #31 #33 #50 #51 #56 — nine superseded by a milestone), **survive 18** (#15 #23 #24 #26 #29 #34 #46 #48 #52 #53 #55 #57 #58 #59 #60 #61 #62 #63 #64 — six deprioritised), **re-scope 10** (#3 #11 #13 #16 #32 #35 #37 #44 #45 #65). New issues: two (M1, M2).

---

## 7. Risks and unknowns

| # | Risk | Status |
|---|---|---|
| R1 | gluetun's WireGuard-mode `VPN_INTERFACE` is `wg0` when the variable is unset; kernelspace mode deletes a same-named link | **verified** (`settings/wireguard.go:196`; *master* `run.go`). The image sets `tun0` (`Dockerfile:79`); the operator renders it unconditionally and rejects overrides |
| R2 | `to <peerCIDR> lookup 51821 pref 90` is needed so provider replies reach `wg0` rather than table 51820's default | **unverified at runtime**; reasoned from *master* `rule.go`/`route.go`; pod-gateway's `suppress_prefixlength 0 pref 50` is the same idea. M4's e2e proves or disproves it |
| R3 | The agent's encapsulated UDP to client pods must leave on eth0 and pass gluetun's `OUTPUT DROP` | **verified** two ways: replies match `OUTPUT ESTABLISHED,RELATED` (`iptables.go:140-145`) and route via `from <podIP> lookup 200 pref 100` (`inbound.go:63-75`); initiated packets are covered by `FIREWALL_OUTBOUND_SUBNETS` (`iptables.go:163-169`, table 199 pref 99) |
| R4 | gluetun rollback flushes rules and sets `FORWARD ACCEPT` | **verified** (`enable.go:44-70`). Not a leak: rule 102 blackholes `iif wg0` regardless. Up to 10 s of *outage* until re-assert |
| R5 | Nested MTU 1320/1320; UDP relies on PMTUD through `wg0`, TCP on the mangle clamp | **unverified** end to end |
| R6 | Node CIDR missing from `localCIDRs` → probe replies enter the tunnel → client pod killed | **verified by reasoning**; pod-gateway documents the same (`settings.sh:10-12`). Mitigated by `MinItems=1` and the worked example |
| R7 | Kernel WireGuard absent on a node | gluetun falls back to userspace and creates `/dev/net/tun` itself (`internal/tun/tun.go`, **not read**); the agent has the same fallback (`wireguard.go:87-134`). kind on GitHub runners has the module |
| R8 | Both images must use the same iptables backend (nft) | **unverified**: gluetun probes `iptables`, `iptables-nft`, `iptables-legacy` in order; Alpine 3.19+ and debian:trixie both ship nft `iptables`, but neither was run |
| R9 | Secret-volume refresh latency gates how fast a new peer appears on the server; the pod-annotation nudge (`:687-709`) bounds it | **present, not measured** |
| R10 | DNS privacy: client DNS resolves via CoreDNS and the node's upstream | **by design**; tracked in #13 |
| R11 | Could not verify: ICMP permitted on GitHub runners for gluetun's check; gluetun reaching `203.0.113.10:80` through its own tunnel in kind; SSA behaviour with duplicate env names; kube-proxy UDP conntrack after the gateway pod is replaced (criterion 5); `generateName` handling under admission (irrelevant to the chosen path); `OwnerReferencesPermissionEnforcement` admission plugin, which would require the sidecar to also hold `pods/finalizers` update — check at M5 | **open** |

---

## 8. Deferred, and how the shape stays additive

- **External clients:** add `serviceType`/`externalAddress` back to `Wireguard`, an optional `privateKeyRef` to `WireguardPeer` for peers that are not pods, and render a wg-quick config for those only. The peer object already is the identity record. wstunnel returns as a sidecar when needed.
- **Webhook injection** (#23, #24, #26): injects the § 3.3 block; the sidecar does not change.
- **Cross-namespace** (#29): `wireguardRef` gains a namespace plus server-side consent; GC via Pod ownerRef still holds because peer and pod stay co-located.
- **DNS through the gateway** (#13): a resolver on `10.8.0.1` forwarding via `tun0`, split to kube-dns for cluster domains; the sidecar rewrites `resolv.conf`. pod-gateway's dnsmasq design is the template.
- **Metrics, IPv6, CNI policy, HA, auto-discovered CIDRs, forwarded ports, gluetun firewall off:** each is a new file plus wiring; none requires touching the two rules or the sidecar's routing.
