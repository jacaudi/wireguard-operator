package resources

import (
	"maps"
	"slices"
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

func TestDeploymentAlwaysEnablesIPForwardingAndReadsNoConfigMap(t *testing.T) {
	spec := buildDeployment(t, testWireguard()).Spec.Template.Spec

	if len(spec.InitContainers) != 1 || spec.InitContainers[0].Name != "sysctl" {
		t.Fatalf("init containers = %v, want exactly [sysctl]", spec.InitContainers)
	}
	sc := spec.InitContainers[0].SecurityContext
	if sc == nil || sc.Privileged == nil || !*sc.Privileged {
		t.Errorf("sysctl init container must be privileged, got %+v", sc)
	}

	agent := spec.Containers[0]
	if len(agent.EnvFrom) != 0 {
		t.Errorf("agent envFrom = %v, want none", agent.EnvFrom)
	}
	if slices.Contains(agent.Command, "--wg-use-userspace-implementation") {
		t.Errorf("agent command %v must not force the userspace implementation", agent.Command)
	}
}
