# Architecture

## The problem

Some workloads need their internet traffic to leave through a commercial VPN provider, while
everything in-cluster — DNS, other pods, Services — keeps working normally. Running a VPN client in
every pod spreads provider credentials across the cluster and makes each pod's kill switch its own
problem. This operator puts one WireGuard server in the workload's namespace instead: pods tunnel
to it by public key, and the server is the single place where traffic meets the provider.

## Two resources

```text
WireguardPeer ──wireguardRef──▶ Wireguard ──owns──▶ Secret <name>, Service <name>-svc, Deployment <name>-dep
  (one per client)               (one per gateway)
```

- **[`Wireguard`](crd-wireguard.md)** — the server. It owns the key Secret, the ClusterIP Service
  and the single-replica Deployment, and it is the controller owner of its peers.
- **[`WireguardPeer`](crd-wireguardpeer.md)** — a client. It carries the public key and receives an
  address. It lives in the server's namespace and is resolved there.

The split follows WireGuard itself: an interface has one key pair and many peers, and a peer is
nothing more than a public key and the addresses routed to it.

## Data path

What works today: a client that holds a registered key reaches the server's `wg0` through the
ClusterIP Service.

```text
client (your WireGuard config) ──UDP 51820──▶ <name>-svc (ClusterIP) ──▶ server pod: agent, wg0 10.8.0.1
```

What is designed: the client becomes a sidecar in the application pod, and the server forwards
peer traffic into a gluetun container in the same pod, which holds the tunnel to the provider.

```text
app pod: app + wg-client sidecar ──wg──▶ server pod: agent wg0 ──▶ gluetun tun0 ──▶ VPN provider
          cluster CIDRs stay on eth0
```

The sidecar and the gluetun hop are designed, not yet implemented — see
[client-sidecar.md](client-sidecar.md) and [gluetun-egress.md](gluetun-egress.md).

## The server pod

- **`sysctl`** (init, privileged) — sets `net.ipv4.ip_forward=1` in the pod's network namespace.
- **`agent`** (`NET_ADMIN`, read-only root filesystem) — creates `wg0` with the kernel module, or
  starts `wireguard-go` when the module is missing. It assigns the first host of `peerCIDR` to
  `wg0`, routes `peerCIDR` to it, and listens on UDP `51820`.
- **State, not commands.** The manager never talks to the agent. It writes the desired state — the
  server's private key and every peer with a key and an address — into the Secret's `state.json`,
  which the pod mounts. The agent watches the file and diffs it against the live device, adding,
  updating and removing peers by public key.
- **Readiness.** `GET /health` on port `8080` re-reads and re-applies the state; it fails when the
  state is missing, invalid or cannot be applied.

## Identity and addresses

- **A peer is its public key.** The client generates its key pair and registers the public half;
  the operator never handles a peer private key. The server's own key pair is generated once by
  the operator and kept in the Secret.
- **Addresses are allocated, not leased.** The operator assigns the first free address in
  `peerCIDR` to each peer that has a key and no `address`, skipping the network address and the
  server's. Allocation runs inside one reconcile over the server's peers, so two new peers in the
  same pass cannot collide. The server routes exactly `<address>/32` to each peer.

## Failure behaviour

Today the server terminates WireGuard and nothing more: it installs no NAT or firewall rules, so
there is no egress for it to leak. The designed gateway fails closed by **routing** rather than by
firewall rules alone:

- **Server side** — traffic arriving on `wg0` that has no route through gluetun's tunnel hits a
  blackhole route instead of the pod's default route.
- **Client side** — the sidecar replaces the pod's default route with `wg0` and keeps only the
  cluster CIDRs on `eth0`, so a missing tunnel means no route to the internet, not a direct one.

Both are designed, not yet implemented.

## Out of scope

- **Clients outside the cluster** — phones, laptops, LoadBalancer or NodePort Services, external
  addresses, wstunnel.
- **Cross-namespace attachment** — a peer and its server share a namespace.
- **IPv6, per-peer egress policy, bandwidth limits, metrics, high availability, and DNS through the
  gateway** — clients keep cluster DNS.
