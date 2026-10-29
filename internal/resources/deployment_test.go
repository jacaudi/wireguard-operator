package resources

import (
	"maps"
	"testing"

	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	return s
}

func testWireguard() *v1alpha1.Wireguard {
	return &v1alpha1.Wireguard{ObjectMeta: metav1.ObjectMeta{Name: "vpn", Namespace: "default"}}
}

func buildDeployment(t *testing.T, wg *v1alpha1.Wireguard) *appsv1.Deployment {
	t.Helper()
	dep, err := NewDeploymentBuilder(testScheme(t), "agent:test", corev1.PullIfNotPresent).ForWireguard(wg)
	if err != nil {
		t.Fatalf("ForWireguard: %v", err)
	}
	return dep
}

func TestAgentContainerExposesOnlyWireguardAndHealthPorts(t *testing.T) {
	containers := buildDeployment(t, testWireguard()).Spec.Template.Spec.Containers
	if len(containers) != 1 || containers[0].Name != "agent" {
		t.Fatalf("containers = %v, want exactly [agent]", containers)
	}

	got := map[string]int32{}
	for _, p := range containers[0].Ports {
		got[p.Name] = p.ContainerPort
	}
	want := map[string]int32{"wireguard": WireguardPort, "http": HTTPPort}
	if !maps.Equal(got, want) {
		t.Errorf("agent ports = %v, want %v", got, want)
	}
}
