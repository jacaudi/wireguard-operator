package controllers

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
	"github.com/jacaudi/wireguard-operator/internal/agent"

	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Ready=False reasons the operator records on a WireguardPeer.
const (
	// ReasonPublicKeyMissing: spec.publicKey is empty.
	ReasonPublicKeyMissing = "PublicKeyMissing"
	// ReasonWireguardNotFound: no Wireguard named spec.wireguardRef exists in the peer's namespace.
	ReasonWireguardNotFound = "WireguardNotFound"
	// ReasonAddressPoolExhausted: the server's peerCIDR has no free address left.
	ReasonAddressPoolExhausted = "AddressPoolExhausted"
)

// notReady is a peer state the server cannot admit: the summary fields and the Ready=False
// condition the operator records for it.
type notReady struct {
	Status           string // status.status: v1alpha1.Pending or v1alpha1.Error
	Message          string // status.message
	Reason           string // Ready condition reason
	ConditionMessage string // Ready condition message
}

// publicKeyMissing is the state of a peer with an empty spec.publicKey.
var publicKeyMissing = notReady{
	Status:           v1alpha1.Pending,
	Message:          "Waiting for spec.publicKey",
	Reason:           ReasonPublicKeyMissing,
	ConditionMessage: "spec.publicKey is empty, so the server cannot admit this peer",
}

// wireguardNotFound is the state of a peer whose spec.wireguardRef names no Wireguard in namespace.
func wireguardNotFound(ref, namespace string) notReady {
	return notReady{
		Status:           v1alpha1.Pending,
		Message:          fmt.Sprintf("Waiting for wireguard resource '%s' to be created", ref),
		Reason:           ReasonWireguardNotFound,
		ConditionMessage: fmt.Sprintf("no Wireguard named %s exists in namespace %s", ref, namespace),
	}
}

// addressPoolExhausted is the not-ready record for a peer that cidr has no free address for.
func addressPoolExhausted(cidr string) notReady {
	return notReady{
		Status:           v1alpha1.Pending,
		Message:          fmt.Sprintf("No free address in %s", cidr),
		Reason:           ReasonAddressPoolExhausted,
		ConditionMessage: fmt.Sprintf("peerCIDR %s has no free address for this peer", cidr),
	}
}

// desiredState renders the agent's state.json content from the server's name, namespace and
// spec and from each peer's name, namespace, spec.publicKey, spec.wireguardRef and
// status.address, sorted by peer name. No other status field, no metadata and no resourceVersion reach it, so
// writing them never changes the rendered state.
func desiredState(wg *v1alpha1.Wireguard, privateKey string, peers []v1alpha1.WireguardPeer) agent.State {
	rendered := make([]v1alpha1.WireguardPeer, len(peers))
	for i, peer := range peers {
		rendered[i] = v1alpha1.WireguardPeer{
			ObjectMeta: metav1.ObjectMeta{Name: peer.Name, Namespace: peer.Namespace},
			Spec:       peer.Spec,
			Status:     v1alpha1.WireguardPeerStatus{Address: peer.Status.Address},
		}
	}
	slices.SortFunc(rendered, func(a, b v1alpha1.WireguardPeer) int { return cmp.Compare(a.Name, b.Name) })

	return agent.State{
		Server: v1alpha1.Wireguard{
			ObjectMeta: metav1.ObjectMeta{Name: wg.Name, Namespace: wg.Namespace},
			Spec:       *wg.Spec.DeepCopy(),
		},
		ServerPrivateKey: privateKey,
		Peers:            rendered,
	}
}

// wireguardForPeer maps a WireguardPeer to the Wireguard its spec.wireguardRef names, in the
// peer's namespace. It returns nil for any other object and for an empty wireguardRef.
func wireguardForPeer(_ context.Context, obj client.Object) []reconcile.Request {
	peer, ok := obj.(*v1alpha1.WireguardPeer)
	if !ok || peer.Spec.WireguardRef == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: peer.Namespace, Name: peer.Spec.WireguardRef}}}
}

// peerStatusMatches reports whether peer already records nr: status.status, status.message,
// no status.address (a peer the server cannot admit holds none), and a Ready condition with Status
// False, nr.Reason, nr.ConditionMessage and ObservedGeneration == peer.Generation.
func peerStatusMatches(peer *v1alpha1.WireguardPeer, nr notReady) bool {
	c := meta.FindStatusCondition(peer.Status.Conditions, ConditionReady)
	return peer.Status.Status == nr.Status &&
		peer.Status.Address == "" &&
		peer.Status.Message == nr.Message &&
		c != nil &&
		c.Status == metav1.ConditionFalse &&
		c.Reason == nr.Reason &&
		c.Message == nr.ConditionMessage &&
		c.ObservedGeneration == peer.Generation
}

// skipPeerWrite reports whether err from a peer status write means "skip this peer for this pass":
// a Conflict (another writer won; its watch event re-enqueues the server) or a NotFound (the peer
// was deleted after the list).
func skipPeerWrite(err error) bool {
	return errors.IsConflict(err) || errors.IsNotFound(err)
}
