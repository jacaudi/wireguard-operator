# Wireguard Operator
<img width="1394" alt="Grafana dashboard screenshot" src="readme/screenshot.png">

Painless deployment of wireguard on kubernetes

## Support

If you are facing any problems please open an [issue](https://github.com/jacaudi/wireguard-operator/issues)

## Tested with
- [x] IBM Cloud Kubernetes Service
- [x] Gcore Labs KMP
  * requires `spec.enableIpForwardOnPodInit: true`
- [x] Google Kubernetes Engine
  * requires `spec.mtu: "1380"`
  * Not compatible with "Container-Optimized OS with containerd" node images
  * Not compatible with autopilot
- [x] DigitalOcean Kubernetes
  * requires `spec.serviceType: "NodePort"`. DigitalOcean LoadBalancer does not support UDP. 
- [ ] Amazon EKS
- [ ] Azure Kubernetes Service
- [ ] ...?

## Architecture 

![alt text](./readme/main.png)

## Features 
* Falls back to userspace implementation of wireguard [wireguard-go](https://github.com/WireGuard/wireguard-go) if wireguard kernal module is missing
* Automatic key generation
* Automatic IP allocation
* Does not need persistance. peer/server keys are stored as k8s secrets and loaded into the wireguard pod
* Exposes a metrics endpoint
* Supports tunneling/traffic obfuscation using [wstunnel](https://github.com/erebe/wstunnel)
* IPv6 support, including IPv6-only peers, through `spec.peerCIDRv6` and `spec.ipv6Only`
* Per-peer egress network policies through `WireguardPeer.spec.egressNetworkPolicies`

## Example

### Server

```
apiVersion: vpn.wireguard-operator.io/v1alpha1
kind: Wireguard
metadata:
  name: "my-cool-vpn"
spec:
  mtu: "1380"
```

### Peer

```
apiVersion: vpn.wireguard-operator.io/v1alpha1
kind: WireguardPeer
metadata:
  name: peer1
spec:
  wireguardRef: "my-cool-vpn"
```

#### Peer configuration

Peer configurations are stored in a Secret named `<wireguard-name>-peer-configs`, one key per peer. Retrieve and decode like this:

```console
kubectl get secret my-cool-vpn-peer-configs -o jsonpath='{.data.peer1}' | base64 -d
```

Use the output to configure your preferred Wireguard client:

```console
[Interface]
PrivateKey = WOhR7uTMAqmZamc1umzfwm8o4ZxLdR5LjDcUYaW/PH8=
Address = 10.8.0.3
DNS = 10.48.0.10, default.svc.cluster.local
MTU = 1380

[Peer]
PublicKey = sO3ZWhnIT8owcdsfwiMRu2D8LzKmae2gUAxAmhx5GTg=
AllowedIPs = 0.0.0.0/0
Endpoint = 32.121.45.102:51820
PersistentKeepalive = 25
```

`PersistentKeepalive` defaults to 25 seconds and is configurable through
`Wireguard.Spec.PersistentKeepalive`. Set it to `0` to omit it.

## How to deploy

> **Note:** this fork has not cut its own release yet, so the commands below install the
> upstream `nccloud` operator, which is diverging from this fork. Features added here —
> `spec.persistentKeepalive`, for example — will not exist in that build. Until a release
> is published (tracked in [#36](https://github.com/jacaudi/wireguard-operator/issues/36)),
> deploy from source with `make deploy`.

### Using provided manifest file
```
kubectl apply -f https://github.com/nccloud/wireguard-operator/releases/download/v2.11.0/release.yaml
```

### Using Helm
```
helm repo add nccloud https://nccloud.github.io/charts
helm install wireguard nccloud/wireguard-operator -n wireguard-system
```

You can use values to further customize the installation:
```
helm install wireguard nccloud/wireguard-operator -n wireguard-system --set nameOverride=wireguard
```

## How to remove
### Using provided manifest file
```
kubectl delete -f https://github.com/nccloud/wireguard-operator/releases/download/v2.11.0/release.yaml
```
### Using Helm
```
helm uninstall wireguard -n wireguard-system
```

## How to collaborate

This project is done on top of [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder), so read about that project
before collaborating. Of course, we are open to external collaborations for this project. For doing it you must fork the
repository, make your changes to the code and open a PR. The code will be reviewed and tested (always)

> We are developers and hate bad code. For that reason we ask you the highest quality on each line of code to improve
> this project on each iteration.
