package controller

import (
	"context"
	"log"
	"time"

	"github.com/madhouselabs/goman/pkg/models"
)

// reconcileBasedOnPhase performs reconciliation based on current phase
func (r *Reconciler) reconcileBasedOnPhase(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	switch resource.Status.Phase {
	case "", models.ClusterPhasePending:
		return r.reconcilePending(ctx, resource)
	case models.ClusterPhaseProvisioning:
		return r.reconcileProvisioning(ctx, resource)
	case models.ClusterPhaseRunning:
		return r.reconcileRunning(ctx, resource)
	case models.ClusterPhaseFailed:
		return r.reconcileFailed(ctx, resource)
	case models.ClusterPhaseDeleting:
		return r.reconcileDeleting(ctx, resource)
	default:
		log.Printf("[PHASE] Unknown phase %s for cluster %s", resource.Status.Phase, resource.Name)
		return &models.ReconcileResult{}, nil
	}
}

// reconcilePending starts provisioning
func (r *Reconciler) reconcilePending(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("[PENDING] Starting provisioning for cluster %s", resource.Name)

	resource.Status.Phase = models.ClusterPhaseProvisioning
	resource.Status.Message = "Starting infrastructure provisioning"

	log.Printf("[PENDING] Transitioning cluster %s to provisioning phase", resource.Name)
	return &models.ReconcileResult{
		Requeue:      true,
		RequeueAfter: PendingRequeuInterval,
	}, nil
}

// reconcileFailed handles failed clusters
func (r *Reconciler) reconcileFailed(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("[FAILED] Cluster %s is in failed state: %s", resource.Name, resource.Status.Message)
	
	// Check if there's a deletion request
	if resource.DeletionTimestamp != nil {
		log.Printf("[FAILED] Deletion requested for failed cluster %s, transitioning to deleting phase", resource.Name)
		resource.Status.Phase = models.ClusterPhaseDeleting
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
		}, nil
	}
	
	// Don't requeue failed clusters unless explicitly requested
	return &models.ReconcileResult{
		Requeue: false,
	}, nil
}