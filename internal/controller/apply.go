// Package controllers implements the Wireguard and WireguardPeer reconcilers.
//
// golangci-lint's revive `package-comments` rule fires on every line of a new file under the
// `new-from-merge-base` ratchet, and no existing file in this package carries a package comment.
// Putting it here, on the new file, satisfies the rule without touching unrelated files.
package controllers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// FieldManager identifies this operator's Server-Side Apply field manager. It is part of the
// on-cluster contract: changing it orphans field ownership on every existing object and leaves
// stale entries in managedFields. Not a tunable.
const FieldManager = "wireguard-operator"

// applyOwned renders an operator-owned object into the cluster with Server-Side Apply. The
// apiserver performs the diff; there is deliberately no client-side comparison.
//
// What the operator ends up owning is what json.Marshal of the built object emits, which is not
// the same as "the fields the builder sets", in either direction. Zero-valued structs are emitted
// despite omitempty -- so the operator owns spec.strategy, the agent container's resources, and
// the pod template's creationTimestamp, none of which any builder sets. Zero-valued scalars with
// omitempty are dropped -- so a nodePort of 0 is not owned, which is what lets the apiserver keep
// its own allocation.
//
// ForceOwnership is unconditional. A conflict is only raised when the applied value differs from
// a value owned by another manager, so the first apply over an object previously written by
// Create/Update succeeds without it -- but the first apply that actually changes such a field
// would fail, and the operator would wedge. Forcing is also what makes the operator converge an
// object a human edited with kubectl, which is the point of this change.
//
// SSA requires apiVersion and kind in the request body, and a typed object's TypeMeta is empty
// after construction, so the GVK is derived from the scheme and stamped on before the call.
// Without this the apiserver rejects the patch with "invalid object type: /, Kind=".
func (r *WireguardReconciler) applyOwned(ctx context.Context, obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, r.Scheme)
	if err != nil {
		return err
	}
	obj.GetObjectKind().SetGroupVersionKind(gvk)
	return r.Patch(ctx, obj, client.Apply,
		client.FieldOwner(FieldManager), client.ForceOwnership)
}

// failDegraded records a Degraded condition and returns the original error for requeue.
//
// Some apply failures are permanent rather than transient -- a Service port colliding with a
// foreign writer's entry, or spec.nodePort set on a ClusterIP Service. Without a status
// condition the operator retries forever with a stale Status and nothing user-visible.
//
// A failure to write the status is logged rather than combined into the returned error: the
// caller needs the original error, and the status write is retried on the next reconcile.
func (r *WireguardReconciler) failDegraded(
	ctx context.Context, req ctrl.Request, err error, reason, msg string,
) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	log.Error(err, msg)

	if serr := r.updateStatus(ctx, req, metav1.Condition{
		Type:    ConditionDegraded,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: err.Error(),
	}); serr != nil {
		log.Error(serr, "failed to record Degraded condition", "reason", reason)
	}
	return ctrl.Result{}, err
}
