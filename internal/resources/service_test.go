package resources

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestServiceIsClusterIPWithOnlyTheWireguardPort(t *testing.T) {
	svc, err := NewServiceBuilder(testScheme(t)).ForWireguard(testWireguard())
	if err != nil {
		t.Fatalf("ForWireguard: %v", err)
	}

	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("type = %q, want %q", svc.Spec.Type, corev1.ServiceTypeClusterIP)
	}
	if len(svc.Annotations) != 0 {
		t.Errorf("annotations = %v, want none", svc.Annotations)
	}
	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("ports = %v, want exactly one", svc.Spec.Ports)
	}
	p := svc.Spec.Ports[0]
	if p.Name != "wireguard" || p.Protocol != corev1.ProtocolUDP || p.Port != WireguardPort || p.NodePort != 0 {
		t.Errorf("port = %+v, want wireguard/UDP/%d with no nodePort", p, WireguardPort)
	}
}
