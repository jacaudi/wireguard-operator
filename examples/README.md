# Example custom resources

Two manifests: a server and one peer of it. Pair them with the reference docs:
[`Wireguard`](../docs/crd-wireguard.md) and [`WireguardPeer`](../docs/crd-wireguardpeer.md).

## Files

| File | What it shows |
|---|---|
| [`01-server.yaml`](01-server.yaml) | `Wireguard` named `vpn` with `spec: {}` — every default: `peerCIDR` `10.8.0.0/24`, ClusterIP Service on UDP `51820` |
| [`02-peer.yaml`](02-peer.yaml) | `WireguardPeer` that references `vpn` and registers a client's public key; the operator fills in `spec.address` |

## Reading order

Apply `01` first, then `02`, into the same namespace:

```sh
kubectl apply -f examples/01-server.yaml
kubectl apply -f examples/02-peer.yaml
kubectl get wireguard,wireguardpeer
```

The peer reads `error` (`Waiting for vpn to be ready`) until the server is `ready`, then `ready`.

> **Keys:** `02-peer.yaml` carries a sample public key. Replace it with the output of
> `wg genkey | wg pubkey` from the client that will connect; the operator never holds the private
> half.

> **Egress:** these examples give a peer a tunnel to the server pod and nothing more. Egress
> through gluetun is designed but not yet implemented (see
> [../docs/gluetun-egress.md](../docs/gluetun-egress.md)).
