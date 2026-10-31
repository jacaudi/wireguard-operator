# `WireguardPeer` reference

A client of a [`Wireguard`](crd-wireguard.md) server, identified by its public key. The operator
allocates the peer an address and the server's agent adds it to `wg0`. **Namespaced**, group
`vpn.wireguard-operator.io`, version `v1alpha1`, no short name. `kubectl get wireguardpeer` prints
`STATUS` (`.status.status`) and `AGE`.

Source of truth: [`api/v1alpha1/wireguardpeer_types.go`](../api/v1alpha1/wireguardpeer_types.go).
See [`examples/02-peer.yaml`](../examples/02-peer.yaml).

## Spec

| Field | Type | Default | Description |
|---|---|---|---|
| `wireguardRef` | string | — | **Required**, minimum length 1. The name of the `Wireguard` this peer belongs to, in the peer's own namespace. |
| `publicKey` | string | — | The peer's WireGuard public key, as printed by `wg pubkey`. Without it the peer gets no address and the server does not admit it. |
| `address` | string | allocated | The peer's IPv4 address on the tunnel, without a prefix length. The operator fills it from the server's `peerCIDR` when it is empty; you can set it yourself. The server routes `<address>/32` to this peer. |

> **The private key never enters the cluster.** Generate the key pair where the client runs and
> register only the public half. The operator neither mints, stores nor reads a peer private key.

## Status (operator-written)

| Field | Description |
|---|---|
| `status` | `pending`, `ready` or `error`. |
| `message` | Why the peer is in that state. See the table below. |
| `conditions` | Standard `metav1.Condition` list, keyed by `type`. Empty for a healthy peer. |

What each state means:

| `status` | `message` | Meaning |
|---|---|---|
| `pending` | `Waiting for wireguard peer to be created` | First reconcile of a new peer. |
| `pending` | `Waiting for spec.publicKey` | No `publicKey`; see `PublicKeyMissing` below. |
| `error` | `Waiting for wireguard resource '<ref>' to be created` | No `Wireguard` named `wireguardRef` exists in this namespace. |
| `error` | `Waiting for <ref> to be ready` | The `Wireguard` exists but is not `ready` yet. |
| `error` | `Duplicate address <ip>: already used by peer <name>` | Another peer of the same server carries the same `address`. |
| `ready` | `Peer configured` | The peer has a key and an address and is part of the server's state. |

## Conditions

| Type | Meaning |
|---|---|
| `Ready` | Written only as `False`, reason `PublicKeyMissing`, message `spec.publicKey is empty, so the server cannot admit this peer`. The operator removes the condition once a `publicKey` is set and the peer has an address. |

## Lifecycle notes

- **Same namespace only.** `wireguardRef` resolves in the peer's own namespace; a peer cannot
  attach to a server elsewhere.
- **Owned by its server.** Once the `Wireguard` is `ready`, the operator sets it as the peer's
  controller owner, so deleting the server deletes its peers.
- **Deleting a peer removes it from `wg0`.** The operator drops it from `state.json`, and the
  agent removes any device peer whose key is no longer listed.
- **Duplicates are reported, not prevented.** A duplicate `address` sets `status: error`, but the
  peer stays in the server's state, and the server's reconcile can set it back to `ready`. On
  `wg0` the `/32` belongs to only one of the two keys. Fix the `address` on one of them.
- **A malformed `publicKey` stalls the whole server.** The API accepts any string. The agent then
  fails to parse the key, applies no peer changes at all, and fails its readiness probe until the
  peer is fixed or deleted. Peer validation is not yet available. Tracked in
  [#37](https://github.com/jacaudi/wireguard-operator/issues/37).
