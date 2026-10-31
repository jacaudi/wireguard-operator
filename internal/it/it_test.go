//go:build e2e
// +build e2e

package it

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("wireguard controller", func() {
	It("wireguard is able to start", func() {
		wireguardYaml :=
			`apiVersion: vpn.wireguard-operator.io/v1alpha1
kind: Wireguard
metadata:
  name: vpn
spec: {}`

		// kubectl apply -f
		output, err := KubectlApply(wireguardYaml, TestNamespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(Equal("wireguard.vpn.wireguard-operator.io/vpn created"))

		wireguardPeerYaml :=
			`apiVersion: vpn.wireguard-operator.io/v1alpha1
kind: WireguardPeer
metadata:
  name: peer20
spec:
  wireguardRef: "vpn"
  publicKey: "PLA+byaz6a5jbnioieB4NEELG7sdq7Wk+3u9Kw0s6zk="

`
		// kubectl apply -f
		output, err = KubectlApply(wireguardPeerYaml, TestNamespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(Equal("wireguardpeer.vpn.wireguard-operator.io/peer20 created"))

		WaitForWireguardToBeReady("vpn", TestNamespace)
		WaitForPeerToBeReady("peer20", TestNamespace)

		// The controller allocated the peer an address from the server's peerCIDR.
		Eventually(func(g Gomega) {
			address, err := KubectlGet("wireguardpeer/peer20", TestNamespace, "{.status.address}")
			g.Expect(err).NotTo(HaveOccurred(), "kubectl get wireguardpeer/peer20: %s", address)
			g.Expect(address).To(Equal("10.8.0.2"))
		}, Timeout, Interval).Should(Succeed())

		// The server published the address a client dials: the Service's ClusterIP and WireGuard port.
		Eventually(func(g Gomega) {
			endpoint, err := KubectlGet("wireguard/vpn", TestNamespace, "{.status.endpoint}")
			g.Expect(err).NotTo(HaveOccurred(), "kubectl get wireguard/vpn: %s", endpoint)
			clusterIP, err := KubectlGet("service/vpn-svc", TestNamespace, "{.spec.clusterIP}")
			g.Expect(err).NotTo(HaveOccurred(), "kubectl get service/vpn-svc: %s", clusterIP)
			g.Expect(endpoint).To(Equal(clusterIP + ":51820"))
		}, Timeout, Interval).Should(Succeed())

		// The agent's wg0 admitted the peer, so the controller's state reached the agent.
		Eventually(func(g Gomega) {
			peers, err := KubectlExec("vpn-dep", "agent", TestNamespace, "wg", "show", "wg0", "peers")
			g.Expect(err).NotTo(HaveOccurred(), "wg show wg0 peers in vpn-dep: %s", peers)
			g.Expect(peers).To(ContainSubstring("PLA+byaz6a5jbnioieB4NEELG7sdq7Wk+3u9Kw0s6zk="))
		}, Timeout, Interval).Should(Succeed())
	})

	It("exposes controller manager metrics", func() {
		// Ensure the controller-manager Deployment is ready (BeforeSuite already waits for it)

		controllerMetricsCheckPod := `
apiVersion: v1
kind: Pod
metadata:
  name: controller-metrics-check
  namespace: wireguard-system
spec:
  restartPolicy: Never
  serviceAccountName: wireguard-metrics-reader
  containers:
  - name: curl
    image: curlimages/curl:8.17.0
    command: ["sh","-c"]
    args:
    - >
      TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token);
      curl -sS -H "Authorization: Bearer $TOKEN" http://wireguard-controller-manager-metrics-service.wireguard-system.svc.cluster.local:8080/metrics |
      grep -E 'controller_runtime_reconcile_total|process_cpu_seconds_total'
`
		output, err := KubectlApply(controllerMetricsCheckPod, "wireguard-system")
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(Equal("pod/controller-metrics-check created"))

		WaitForPodSucceeded("controller-metrics-check", "wireguard-system")

		logs, err := KubectlLogs("controller-metrics-check", "wireguard-system")
		Expect(err).NotTo(HaveOccurred())
		Expect(logs).To(ContainSubstring("controller_runtime_reconcile_total"))
	})
})
