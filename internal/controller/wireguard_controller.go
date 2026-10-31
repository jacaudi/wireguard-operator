/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/jacaudi/wireguard-operator/api/v1alpha1"
	"github.com/jacaudi/wireguard-operator/internal/agent"
	"github.com/jacaudi/wireguard-operator/internal/ipam"
	"github.com/jacaudi/wireguard-operator/internal/resources"

	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// Standard condition types for Wireguard
const (
	ConditionReady       = "Ready"
	ConditionProgressing = "Progressing"
	ConditionDegraded    = "Degraded"
)

// WireguardReconciler reconciles a Wireguard object
type WireguardReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	AgentImage           string
	AgentImagePullPolicy corev1.PullPolicy

	// Resource builders
	secretBuilder     *resources.SecretBuilder
	serviceBuilder    *resources.ServiceBuilder
	deploymentBuilder *resources.DeploymentBuilder
	ipAllocator       *ipam.Allocator
}

func (r *WireguardReconciler) getWireguardPeers(ctx context.Context, req ctrl.Request) (*v1alpha1.WireguardPeerList, error) {
	peers := &v1alpha1.WireguardPeerList{}
	if err := r.List(ctx, peers, client.InNamespace(req.Namespace)); err != nil {
		return nil, err
	}

	relatedPeers := &v1alpha1.WireguardPeerList{}

	for _, peer := range peers.Items {
		if peer.Spec.WireguardRef == req.Name {
			relatedPeers.Items = append(relatedPeers.Items, peer)
		}
	}

	return relatedPeers, nil
}

func (r *WireguardReconciler) updateStatus(ctx context.Context, req ctrl.Request, cond metav1.Condition) error {
	latest := &v1alpha1.Wireguard{}
	if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Ensure transition time and observed generation are set
	if cond.LastTransitionTime.IsZero() {
		cond.LastTransitionTime = metav1.Now()
	}
	cond.ObservedGeneration = latest.GetGeneration()

	meta.SetStatusCondition(&latest.Status.Conditions, cond)

	// Ready=True is written only once every apply in the reconcile has succeeded, so it also clears
	// a Degraded left by an earlier failure. It must ride in this same write: a separate
	// updateStatus call for Degraded would set the summary below to Error, and a second write that
	// hits a Conflict is swallowed, so the clearing could be lost without trace.
	if cond.Type == ConditionReady && cond.Status == metav1.ConditionTrue {
		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               ConditionDegraded,
			Status:             metav1.ConditionFalse,
			Reason:             "Reconciled",
			Message:            "All owned resources applied",
			ObservedGeneration: cond.ObservedGeneration,
		})
	}

	// Backward-compatible summary fields derived from condition type
	switch cond.Type {
	case ConditionReady:
		latest.Status.Status = v1alpha1.Ready
	case ConditionDegraded:
		latest.Status.Status = v1alpha1.Error
	default:
		latest.Status.Status = v1alpha1.Pending
	}
	latest.Status.Message = cond.Message

	if err := r.Status().Update(ctx, latest); err != nil {
		if errors.IsNotFound(err) || errors.IsConflict(err) {
			// Object deleted or updated concurrently; ignore and let reconcile retry
			return nil
		}
		return err
	}
	return nil
}

// effectivePeerCIDR4 returns the effective IPv4 peer CIDR for a Wireguard instance.
func effectivePeerCIDR4(wg *v1alpha1.Wireguard) string {
	if wg.Spec.PeerCIDR != "" {
		return wg.Spec.PeerCIDR
	}
	return ipam.DefaultPeerCIDR4
}

// updateWireguardPeers allocates a status.address to every peer of this Wireguard that has a
// publicKey but no address, and marks each such peer Ready. A peer without a publicKey gets no
// address: it is marked Ready=False with reason PublicKeyMissing.
func (r *WireguardReconciler) updateWireguardPeers(ctx context.Context, req ctrl.Request, wireguard *v1alpha1.Wireguard) error {
	peers, err := r.getWireguardPeers(ctx, req)
	if err != nil {
		return err
	}

	cidr4 := effectivePeerCIDR4(wireguard)
	usedIps4 := r.ipAllocator.GetUsedIPs(cidr4, peers)

	for _, peer := range peers.Items {
		if peer.Spec.PublicKey == "" {
			if err := r.markPeerNotReady(ctx, &peer, publicKeyMissing); err != nil {
				// A skipped peer is this peer's retry, not the loop's: whoever won the write,
				// or the deletion, also emits a watch event, which re-enqueues the Wireguard.
				if skipPeerWrite(err) {
					continue
				}
				return err
			}
			continue
		}

		allocated := peer.Status.Address == ""
		if allocated {
			ip, err := r.ipAllocator.AllocateIP(cidr4, usedIps4)
			if err != nil {
				return err
			}
			peer.Status.Address = ip
			// Reserved even if the write below is lost to a Conflict, so the next peer in this
			// pass cannot take the same address.
			usedIps4 = append(usedIps4, ip)
		}

		// A peer that was keyless still carries Ready=False/PublicKeyMissing; clear it.
		if !allocated && peer.Status.Status == v1alpha1.Ready && meta.FindStatusCondition(peer.Status.Conditions, ConditionReady) == nil {
			continue
		}
		peer.Status.Status = v1alpha1.Ready
		peer.Status.Message = "Peer configured"
		meta.RemoveStatusCondition(&peer.Status.Conditions, ConditionReady)
		if err := r.Status().Update(ctx, &peer); err != nil {
			if skipPeerWrite(err) {
				continue
			}
			return err
		}
	}

	return nil
}

// markPeerNotReady records nr on peer with one status write, skipped when peerStatusMatches.
func (r *WireguardReconciler) markPeerNotReady(ctx context.Context, peer *v1alpha1.WireguardPeer, nr notReady) error {
	if peerStatusMatches(peer, nr) {
		return nil
	}
	peer.Status.Status = nr.Status
	peer.Status.Message = nr.Message
	peer.Status.Address = "" // a peer the server cannot admit releases its address
	meta.SetStatusCondition(&peer.Status.Conditions, metav1.Condition{
		Type:               ConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             nr.Reason,
		Message:            nr.ConditionMessage,
		ObservedGeneration: peer.Generation,
	})
	return r.Status().Update(ctx, peer)
}

// markPeersWireguardNotFound records WireguardNotFound on every peer in req.Namespace whose
// wireguardRef is req.Name.
func (r *WireguardReconciler) markPeersWireguardNotFound(ctx context.Context, req ctrl.Request) error {
	peers, err := r.getWireguardPeers(ctx, req)
	if err != nil {
		return err
	}
	nr := wireguardNotFound(req.Name, req.Namespace)
	for i := range peers.Items {
		if err := r.markPeerNotReady(ctx, &peers.Items[i], nr); err != nil {
			if skipPeerWrite(err) {
				continue
			}
			return err
		}
	}
	return nil
}

//+kubebuilder:rbac:groups=vpn.wireguard-operator.io,resources=wireguards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpn.wireguard-operator.io,resources=wireguards/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpn.wireguard-operator.io,resources=wireguards/finalizers,verbs=update
// +kubebuilder:rbac:groups=vpn.wireguard-operator.io,resources=wireguardpeers,verbs=get;list;watch
// +kubebuilder:rbac:groups=vpn.wireguard-operator.io,resources=wireguardpeers/status,verbs=get;update;patch

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Wireguard object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	log.Info("loaded the following wireguard image:" + r.AgentImage)

	wireguard := &v1alpha1.Wireguard{}
	log.Info(req.Name)
	err := r.Get(ctx, req.NamespacedName, wireguard)
	if err != nil {
		if errors.IsNotFound(err) {
			// The Wireguard is gone: record WireguardNotFound on the peers that still name it.
			log.Info("wireguard resource not found; marking its peers WireguardNotFound")
			return ctrl.Result{}, r.markPeersWireguardNotFound(ctx, req)
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get wireguard")
		return ctrl.Result{}, err
	}

	log.Info("processing " + wireguard.Name)

	if wireguard.Status.Status == "" {
		err = r.updateStatus(ctx, req, metav1.Condition{Type: ConditionProgressing, Status: metav1.ConditionTrue, Reason: "FetchingStatus", Message: "Fetching Wireguard status"})

		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// wireguardpeer
	peers := &v1alpha1.WireguardPeerList{}
	// TODO add a label to wireguardpeers and then filter by label here to only get peers of the wg instance we need.
	if err := r.List(ctx, peers, client.InNamespace(req.Namespace)); err != nil {
		log.Error(err, "Failed to fetch list of peers")
		return ctrl.Result{}, err
	}

	var filteredPeers []v1alpha1.WireguardPeer
	for _, peer := range peers.Items {
		if peer.Spec.WireguardRef != wireguard.Name {
			continue
		}
		if peer.Spec.PublicKey == "" {
			continue
		}

		if peer.Status.Address == "" {
			continue
		}

		filteredPeers = append(filteredPeers, peer)
	}

	// service
	//
	// Every field the builder renders converges. The server-assigned clusterIP survives because
	// the builder never renders it, so the operator never takes ownership of the allocation.
	desiredSvc, err := r.serviceBuilder.ForWireguard(wireguard)
	if err != nil {
		return r.failDegraded(ctx, req, err, "ServiceBuildFailed", "Failed to build service")
	}
	if err := r.applyOwned(ctx, desiredSvc); err != nil {
		return r.failDegraded(ctx, req, err, "ServiceApplyFailed", "Failed to apply service")
	}

	// fetch secret
	secret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: wireguard.Name, Namespace: wireguard.Namespace}, secret)
	// secret already created
	if err == nil {
		privateKey := string(secret.Data["privateKey"])

		state := agent.State{
			Server:           *wireguard.DeepCopy(),
			ServerPrivateKey: privateKey,
			Peers:            filteredPeers,
		}

		b, err := json.Marshal(state)
		if err != nil {
			log.Error(err, "Failed to save state to secret")
			return ctrl.Result{}, err
		}

		if !bytes.Equal(b, secret.Data["state.json"]) {
			log.Info("Updating secret with new config")
			publicKey := string(secret.Data["publicKey"])

			updatedSecret, err := r.secretBuilder.ForWireguard(wireguard, b, privateKey, publicKey)
			if err != nil {
				log.Error(err, "Failed to build secret with new config")
				return ctrl.Result{}, err
			}
			if err := r.Update(ctx, updatedSecret); err != nil {
				log.Error(err, "Failed to update secret with new config")
				return ctrl.Result{}, err
			}

			pods := &corev1.PodList{}
			if err := r.List(ctx, pods, client.MatchingLabels{"app": "wireguard", "instance": wireguard.Name}); err != nil {
				log.Error(err, "Failed to fetch list of pods")
				return ctrl.Result{}, err
			}

			for _, pod := range pods.Items {
				if pod.Annotations == nil {
					pod.Annotations = make(map[string]string)
				}
				// this is needed to force k8s to push the new secret to the pod
				pod.Annotations["wgConfigLastUpdated"] = time.Now().Format("2006-01-02T15-04-05")
				if err := r.Update(ctx, &pod); err != nil {
					if errors.IsConflict(err) {
						// ignore conflict; we'll see the updated pod on next reconcile
					} else {
						log.Error(err, "Failed to update pod")
						return ctrl.Result{}, err
					}
				}

				log.Info("updated pod")
			}

		}

	}
	// secret not yet created
	if err != nil && errors.IsNotFound(err) {

		key, err := wgtypes.GeneratePrivateKey()
		if err != nil {
			log.Error(err, "Failed to generate private key")
			return ctrl.Result{}, err
		}

		privateKey := key.String()
		publicKey := key.PublicKey().String()

		state := agent.State{
			Server:           *wireguard.DeepCopy(),
			ServerPrivateKey: privateKey,
			Peers:            filteredPeers,
		}

		b, err := json.Marshal(state)
		if err != nil {
			log.Error(err, "Failed to save state to secret")
			return ctrl.Result{}, err
		}

		secret, err := r.secretBuilder.ForWireguard(wireguard, b, privateKey, publicKey)
		if err != nil {
			log.Error(err, "Failed to build secret", "wireguard.Name", wireguard.Name)
			return ctrl.Result{}, err
		}

		log.Info("Creating a new secret", "secret.Namespace", secret.Namespace, "secret.Name", secret.Name)

		if err := r.Create(ctx, secret); err != nil {
			log.Error(err, "Failed to create new secret", "secret.Namespace", secret.Namespace, "secret.Name", secret.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	} else if err != nil {
		log.Error(err, "Failed to get secret")
		return ctrl.Result{}, err
	}

	// deployment
	//
	// One build, one apply. The apiserver diffs the applied field set against the stored
	// object, so every field the builder renders converges without a bespoke drift check.
	desiredDep, err := r.deploymentBuilder.ForWireguard(wireguard)
	if err != nil {
		return r.failDegraded(ctx, req, err, "DeploymentBuildFailed", "Failed to build deployment")
	}
	if err := r.applyOwned(ctx, desiredDep); err != nil {
		return r.failDegraded(ctx, req, err, "DeploymentApplyFailed", "Failed to apply deployment")
	}

	if err := r.updateWireguardPeers(ctx, req, wireguard); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Updated related peers", "wireguard.Namespace", wireguard.Namespace, "wireguard.Name", wireguard.Name)

	err = r.updateStatus(ctx, req, metav1.Condition{Type: ConditionReady, Status: metav1.ConditionTrue, Reason: "Active", Message: "VPN is active!"})

	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WireguardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize resource builders
	r.secretBuilder = resources.NewSecretBuilder(r.Scheme)
	r.serviceBuilder = resources.NewServiceBuilder(r.Scheme)
	r.deploymentBuilder = resources.NewDeploymentBuilder(r.Scheme, r.AgentImage, r.AgentImagePullPolicy)
	r.ipAllocator = ipam.NewAllocator()

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Wireguard{}).
		Watches(&v1alpha1.WireguardPeer{}, handler.EnqueueRequestsFromMapFunc(wireguardForPeer)).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
