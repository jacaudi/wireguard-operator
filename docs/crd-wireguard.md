# `Wireguard` reference

The server. One `Wireguard` yields one WireGuard server pod, its key Secret and a ClusterIP
Service, all in the CR's namespace. **Namespaced**, group `vpn.wireguard-operator.io`, version
`v1alpha1`, no short name. `kubectl get wireguard` prints `STATUS` (`.status.status`) and `AGE`.

Source of truth: [`api/v1alpha1/wireguard_types.go`](../api/v1alpha1/wireguard_types.go). See
[`examples/01-server.yaml`](../examples/01-server.yaml).

## Spec

Every field is optional; `spec: {}` is a working server.

| Field | Type | Default | Description |
|---|---|---|---|
| `peerCIDR` | string | `10.8.0.0/24` | IPv4 range peer addresses are allocated from. It must be a canonical IPv4 prefix (no host bits set) of `/30` or shorter; anything else sets `Degraded` with reason `InvalidPeerCIDR`. The server's `wg0` takes the first host (`10.8.0.1` by default); the network address and that host are never allocated to a peer. The default is applied by the operator when the field is empty; the field itself stays empty. |
| `nodeSelector` | map[string]string | — | Copied to the server pod's `nodeSelector`. |
| `tolerations` | []`Toleration` | — | Copied to the server pod's `tolerations`. |
| `agent.resources` | `ResourceRequirements` | — | Requests and limits for the `agent` container. The `sysctl` init container gets none. |

## Status (operator-written)

| Field | Description |
|---|---|
| `status` | Summary: `pending`, `ready` or `error`. Derived from the condition the operator wrote last — `Ready` → `ready`, `Degraded` → `error`, anything else → `pending`. |
| `message` | The message of the condition the operator wrote last, for example `VPN is active!`. |
| `endpoint` | The address a client dials: the Service's ClusterIP and the WireGuard port, `<ClusterIP>:51820`. Written with `Ready=True`; keeps its last value while the server is `Degraded`. |
| `publicKey` | The server's WireGuard public key. Written with `Ready=True`; keeps its last value while the server is `Degraded`. |
| `conditions` | Standard `metav1.Condition` list, keyed by `type`. See below. |

## Conditions

| Type | Meaning |
|---|---|
| `Ready` | `True`, reason `Active`, once the Service, key Secret and Deployment are applied and every admissible peer was processed. A peer the pool cannot serve reports `AddressPoolExhausted` and does not hold the server back. It does **not** wait for the server pod to pass its readiness probe. |
| `Progressing` | `True`, reason `FetchingStatus`, written on the first reconcile of a new object. The operator does not set it back to `False` today. |
| `Degraded` | `True` when `peerCIDR` is invalid, or when building or applying an owned object fails. Reasons: `InvalidPeerCIDR`, `ServiceBuildFailed`, `ServiceApplyFailed`, `DeploymentBuildFailed`, `DeploymentApplyFailed`; the message is the error. Set to `False`, reason `Reconciled`, in the same write that sets `Ready=True`. |

## Owned objects

All three carry the labels `app: wireguard` and `instance: <name>`, are owned by the `Wireguard`,
and are deleted with it.

| Kind | Name | Contents |
|---|---|---|
| `Secret` | `<name>` | `privateKey` and `publicKey` (the server key pair), and `state.json` (the server and its admitted peers, those with a valid key and an address, read by the agent). |
| `Service` | `<name>-svc` | `ClusterIP`, one port `wireguard`, UDP `51820`. |
| `Deployment` | `<name>-dep` | One replica. Init container `sysctl` (privileged, sets `net.ipv4.ip_forward=1`); container `agent` (`NET_ADMIN`, read-only root filesystem, UDP `51820`, readiness `GET /health` and liveness TCP on `8080`). Seccomp `RuntimeDefault`; no service-account token. |

## Lifecycle notes

- **The server key is minted once.** The operator generates the key pair when the Secret does not
  exist and reuses it afterwards. Deleting the Secret makes the next reconcile mint a new key,
  and every client then needs the new public key.
- **Peer changes reach the server without a restart.** When the desired state changes, the
  operator rewrites `state.json` and stamps a `wgConfigLastUpdated` annotation on the server pods
  so the kubelet refreshes the mounted Secret sooner; the agent watches the file and applies the
  difference.
- **Set `peerCIDR` before you add peers.** Changing it re-allocates every peer whose address falls
  outside the new range, and a running server keeps its old `wg0` address until its pod is
  replaced.
- **Peers outlive their server.** Peers created by this operator version carry no
  ownerReference to their `Wireguard` and are not deleted with it. Peers created by an earlier
  version keep theirs.
- **Server-Side Apply.** The operator applies the Service and Deployment as field manager
  `wireguard-operator`. Fields it renders converge on every reconcile; fields it does not render,
  such as the Service's allocated `clusterIP`, are left alone.
- **No internet egress yet.** The server pod enables IP forwarding but installs no NAT or
  firewall rules. Egress through gluetun is designed, not yet implemented — see
  [gluetun-egress.md](gluetun-egress.md).
