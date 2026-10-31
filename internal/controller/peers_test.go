package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
	"github.com/jacaudi/wireguard-operator/internal/agent"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestWireguardForPeer(t *testing.T) {
	tests := []struct {
		name string
		obj  client.Object
		want []reconcile.Request
	}{
		{
			name: "peer maps to the Wireguard its wireguardRef names, in the peer's namespace",
			obj: &v1alpha1.WireguardPeer{
				ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "media"},
				Spec:       v1alpha1.WireguardPeerSpec{WireguardRef: "egress"},
			},
			want: []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: "media", Name: "egress"}}},
		},
		{
			name: "empty wireguardRef maps to nothing",
			obj:  &v1alpha1.WireguardPeer{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "media"}},
			want: nil,
		},
		{
			name: "another kind maps to nothing",
			obj:  &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "media"}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wireguardForPeer(t.Context(), tt.obj)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("wireguardForPeer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWireguardNotFound(t *testing.T) {
	want := notReady{
		Status:           "pending",
		Message:          "Waiting for wireguard resource 'vpn' to be created",
		Reason:           "WireguardNotFound",
		ConditionMessage: "no Wireguard named vpn exists in namespace default",
	}
	if got := wireguardNotFound("vpn", "default"); !reflect.DeepEqual(got, want) {
		t.Errorf("wireguardNotFound() = %+v, want %+v", got, want)
	}
}

func TestPeerStatusMatches(t *testing.T) {
	// matching returns a peer that records publicKeyMissing at generation 2.
	matching := func() *v1alpha1.WireguardPeer {
		return &v1alpha1.WireguardPeer{
			ObjectMeta: metav1.ObjectMeta{Generation: 2},
			Status: v1alpha1.WireguardPeerStatus{
				Status:  publicKeyMissing.Status,
				Message: publicKeyMissing.Message,
				Conditions: []metav1.Condition{{
					Type:               ConditionReady,
					Status:             metav1.ConditionFalse,
					Reason:             ReasonPublicKeyMissing,
					Message:            publicKeyMissing.ConditionMessage,
					ObservedGeneration: 2,
				}},
			},
		}
	}

	tests := []struct {
		name   string
		mutate func(*v1alpha1.WireguardPeer)
		want   bool
	}{
		{name: "matches", mutate: func(*v1alpha1.WireguardPeer) {}, want: true},
		{name: "no condition", mutate: func(p *v1alpha1.WireguardPeer) { p.Status.Conditions = nil }, want: false},
		{
			name:   "stale observedGeneration",
			mutate: func(p *v1alpha1.WireguardPeer) { p.Status.Conditions[0].ObservedGeneration = 1 },
			want:   false,
		},
		{
			name:   "other reason",
			mutate: func(p *v1alpha1.WireguardPeer) { p.Status.Conditions[0].Reason = ReasonWireguardNotFound },
			want:   false,
		},
		{
			name:   "other summary status",
			mutate: func(p *v1alpha1.WireguardPeer) { p.Status.Status = v1alpha1.Ready },
			want:   false,
		},
		{
			name:   "address not yet released",
			mutate: func(p *v1alpha1.WireguardPeer) { p.Status.Address = "10.8.0.9" },
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer := matching()
			tt.mutate(peer)
			if got := peerStatusMatches(peer, publicKeyMissing); got != tt.want {
				t.Errorf("peerStatusMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSkipPeerWrite(t *testing.T) {
	gr := schema.GroupResource{Group: "vpn.wireguard-operator.io", Resource: "wireguardpeers"}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "conflict", err: apierrors.NewConflict(gr, "p", errors.New("stale")), want: true},
		{name: "not found", err: apierrors.NewNotFound(gr, "p"), want: true},
		{name: "bad request", err: apierrors.NewBadRequest("x"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := skipPeerWrite(tt.err); got != tt.want {
				t.Errorf("skipPeerWrite(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDesiredStateIgnoresStatusAndMetadata(t *testing.T) {
	const privateKey = "PLA+byaz6a5jbnioieB4NEELG7sdq7Wk+3u9Kw0s6zk="

	wg := &v1alpha1.Wireguard{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "vpn",
			Namespace:       "media",
			ResourceVersion: "7",
			Annotations:     map[string]string{"probe.test/nudge": "a"},
		},
		Spec:   v1alpha1.WireguardSpec{PeerCIDR: "10.9.0.0/24"},
		Status: v1alpha1.WireguardStatus{Status: v1alpha1.Ready},
	}
	// peer returns a peer with every field the server must NOT render set, so the test can vary them.
	peer := func(name, publicKey, address, resourceVersion string) v1alpha1.WireguardPeer {
		return v1alpha1.WireguardPeer{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       "media",
				ResourceVersion: resourceVersion,
				Labels:          map[string]string{"probe.test/nudge": resourceVersion},
				ManagedFields:   []metav1.ManagedFieldsEntry{{Manager: "manager", Operation: metav1.ManagedFieldsOperationUpdate}},
			},
			Spec: v1alpha1.WireguardPeerSpec{WireguardRef: "vpn", PublicKey: publicKey},
			Status: v1alpha1.WireguardPeerStatus{
				Address: address,
				Conditions: []metav1.Condition{{
					Type:   ConditionReady,
					Status: metav1.ConditionTrue,
					Reason: "Observed" + resourceVersion,
				}},
			},
		}
	}
	peers := []v1alpha1.WireguardPeer{
		peer("b", "keyB", "10.9.0.3", "11"),
		peer("a", "keyA", "10.9.0.2", "12"),
	}

	want := agent.State{
		Server: v1alpha1.Wireguard{
			ObjectMeta: metav1.ObjectMeta{Name: "vpn", Namespace: "media"},
			Spec:       v1alpha1.WireguardSpec{PeerCIDR: "10.9.0.0/24"},
		},
		ServerPrivateKey: privateKey,
		Peers: []v1alpha1.WireguardPeer{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "media"},
				Spec:       v1alpha1.WireguardPeerSpec{WireguardRef: "vpn", PublicKey: "keyA"},
				Status:     v1alpha1.WireguardPeerStatus{Address: "10.9.0.2"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "media"},
				Spec:       v1alpha1.WireguardPeerSpec{WireguardRef: "vpn", PublicKey: "keyB"},
				Status:     v1alpha1.WireguardPeerStatus{Address: "10.9.0.3"},
			},
		},
	}

	got := desiredState(wg, privateKey, peers)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("desiredState() = %+v, want %+v", got, want)
	}
	if peers[0].Name != "b" || peers[1].Name != "a" {
		t.Errorf("desiredState() reordered its input: got %q then %q, want b then a", peers[0].Name, peers[1].Name)
	}

	// Changing every field the agent does not read must not change a byte of the rendered state.
	before, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	wg.ResourceVersion = "8"
	wg.Annotations["probe.test/nudge"] = "b"
	wg.Status.Status = v1alpha1.Pending
	for i := range peers {
		peers[i].ResourceVersion += "9"
		peers[i].Labels["probe.test/nudge"] = "changed"
		peers[i].Status.Conditions[0].Reason = "Changed"
	}
	after, err := json.Marshal(desiredState(wg, privateKey, peers))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("state.json changed with fields the agent does not read:\n before %s\n after  %s", before, after)
	}
}

func TestAddressPoolExhausted(t *testing.T) {
	want := notReady{
		Status:           "pending",
		Message:          "No free address in 10.8.0.0/30",
		Reason:           "AddressPoolExhausted",
		ConditionMessage: "peerCIDR 10.8.0.0/30 has no free address for this peer",
	}
	if got := addressPoolExhausted("10.8.0.0/30"); !reflect.DeepEqual(got, want) {
		t.Errorf("addressPoolExhausted() = %+v, want %+v", got, want)
	}
}
