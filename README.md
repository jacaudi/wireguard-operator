# Wireguard Operator

Painless deployment of wireguard on kubernetes

## Support

If you are facing any problems please open an [issue](https://github.com/jacaudi/wireguard-operator/issues)

## Features 
* Falls back to userspace implementation of wireguard [wireguard-go](https://github.com/WireGuard/wireguard-go) if wireguard kernal module is missing
* Automatic server key generation
* Automatic IP allocation for peers
* Does not need persistance. The server key is stored as a k8s secret and loaded into the wireguard pod
* The server is reachable in-cluster only, through a ClusterIP Service on UDP 51820

## Example

### Server

```
apiVersion: vpn.wireguard-operator.io/v1alpha1
kind: Wireguard
metadata:
  name: "my-cool-vpn"
spec: {}
```

### Peer

```
apiVersion: vpn.wireguard-operator.io/v1alpha1
kind: WireguardPeer
metadata:
  name: peer1
spec:
  wireguardRef: "my-cool-vpn"
  # The public half of a key the client generated itself: wg genkey | wg pubkey
  publicKey: "v5Q+oRNBfxafhiAR9bXGEhRsRFfEFb7XsFV8DWGrykQ="
```

The operator stores no client configuration and no client private key: each peer
generates its own key and registers only the public half. The in-pod client that
consumes this is designed in
[`docs/designs/2026-10-29-minimal-vpn-gateway-design.md`](docs/designs/2026-10-29-minimal-vpn-gateway-design.md).

## How to deploy

> **Note:** this fork has not published an install artifact yet. An OCI Helm chart is the
> planned install path, tracked in
> [#48](https://github.com/jacaudi/wireguard-operator/issues/48). Until it ships, deploy
> from source.
>
> Earlier revisions of this README pointed at upstream's release and chart. Those
> install the diverging upstream operator, which is a different product from this fork,
> so they have been removed rather than repointed.

```
make deploy
```

## How to remove

```
make undeploy
```

## How to collaborate

This project is done on top of [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder), so read about that project
before collaborating. Of course, we are open to external collaborations for this project. For doing it you must fork the
repository, make your changes to the code and open a PR. The code will be reviewed and tested (always)

> We are developers and hate bad code. For that reason we ask you the highest quality on each line of code to improve
> this project on each iteration.
