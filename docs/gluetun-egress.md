# Egress through gluetun

> **Status: designed, not yet implemented.** The server pod runs WireGuard only; traffic is **not**
> yet forwarded through gluetun, and the `Wireguard` CRD has no `gluetun` or `localCIDRs` field.
> This page describes the intended design so the data path is clear; do not rely on it for privacy
> yet. Tracked in [#3](https://github.com/jacaudi/wireguard-operator/issues/3).

## Today: the server terminates WireGuard and stops there

The server pod enables IPv4 forwarding but installs no NAT, firewall or policy-routing rules. Peer
traffic bound for the internet is not masqueraded and has no supported exit. Use the server for
peer-to-server traffic only.

## Planned: a gluetun sidecar in the server pod

The server pod gains a [gluetun](https://github.com/qdm12/gluetun) container. The agent and
gluetun share one network namespace, so peer traffic arriving on `wg0` is forwarded into
gluetun's `tun0` and on to the provider.

- **Configuration is gluetun's own.** A new `spec.gluetun` passes environment variables through to
  the container, with provider credentials referenced from a Secret by `envFrom`, never written
  inline. There is no typed provider schema: gluetun's variables are the contract.
- **The operator owns the variables the routing depends on.** It renders `VPN_TYPE`,
  `VPN_INTERFACE`, the firewall inputs and outbound subnets, the health-server address and the
  tunnel MTU itself, and reports `Degraded` if you set any of them.
- **Cluster traffic stays in the cluster.** A new required `spec.localCIDRs` lists the pod, Service
  and node CIDRs. Clients route those over the CNI, and gluetun allows them as outbound subnets.

## Planned: fail closed by routing

Two policy-routing rules and two one-route tables carry the safety property:

- traffic for `peerCIDR` returns to `wg0`;
- anything else that arrived on `wg0` and is not routed by gluetun's tunnel table hits a blackhole
  route.

With the tunnel down, gluetun crashed, or gluetun's firewall rolled back, peer traffic is dropped;
the pod's own default route is never reachable from `wg0`. A small set of iptables rules — forward
`wg0` to `tun0`, masquerade, clamp MSS — handles availability and is re-asserted every few
seconds, so a gluetun restart costs an outage, not a leak.

## Planned: readiness follows the tunnel

The agent's `/health` also polls gluetun's health server. An unhealthy tunnel makes the server pod
not ready, the Service drops its only endpoint, and clients stop handshaking. The `Wireguard`
reports `Ready=False` while its Deployment is not ready.
