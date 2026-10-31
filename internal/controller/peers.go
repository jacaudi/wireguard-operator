package controllers

import (
	"context"
	"fmt"

	"github.com/jacaudi/wireguard-operator/api/v1alpha1"

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
// and a Ready condition with Status False, nr.Reason, nr.ConditionMessage and
// ObservedGeneration == peer.Generation.
func peerStatusMatches(peer *v1alpha1.WireguardPeer, nr notReady) bool {
	c := meta.FindStatusCondition(peer.Status.Conditions, ConditionReady)
	return peer.Status.Status == nr.Status &&
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
