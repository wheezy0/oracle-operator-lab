/*
Copyright 2026.

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

package controller

import (
	"context"
	"errors"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	oraclev1alpha1 "dboperator.io/oracle-operator/api/v1alpha1"
)

const finalizerName = "oracle.dboperator.io/finalizer"

// OracleDatabaseReconciler reconciles a OracleDatabase object
type OracleDatabaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	APIURL string
}

// +kubebuilder:rbac:groups=oracle.dboperator.io,resources=oracledatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=oracle.dboperator.io,resources=oracledatabases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=oracle.dboperator.io,resources=oracledatabases/finalizers,verbs=update

func (r *OracleDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var db oraclev1alpha1.OracleDatabase
	if err := r.Get(ctx, req.NamespacedName, &db); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	api := NewAPIClient(r.APIURL)

	// -----------------------------------------------------------------------
	// Deletion: if the resource is being deleted, clean up on the API side
	// then remove our finalizer so k8s can complete the deletion.
	// -----------------------------------------------------------------------
	if !db.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&db, finalizerName) {
			if db.Status.DbID != "" {
				if err := api.Delete(db.Status.DbID); err != nil {
					log.Error(err, "failed to delete database from API", "dbID", db.Status.DbID)
					// Don't block deletion on API errors — log and proceed.
				}
			}
			controllerutil.RemoveFinalizer(&db, finalizerName)
			if err := r.Update(ctx, &db); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// -----------------------------------------------------------------------
	// Ensure our finalizer is registered so deletion goes through Reconcile.
	// -----------------------------------------------------------------------
	if !controllerutil.ContainsFinalizer(&db, finalizerName) {
		controllerutil.AddFinalizer(&db, finalizerName)
		if err := r.Update(ctx, &db); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// -----------------------------------------------------------------------
	// Build the API request payload from the resource spec.
	// -----------------------------------------------------------------------
	apiReq := DBRequest{
		DbName:       db.Spec.DbName,
		Owner:        db.Spec.Owner,
		Version:      db.Spec.Version,
		CharacterSet: db.Spec.CharacterSet,
		SizeGB:       db.Spec.SizeGB,
		ServiceName:  db.Spec.ServiceName,
		PdbName:      db.Spec.PdbName,
		K8sName:      db.Name,
		K8sNamespace: db.Namespace,
	}

	// -----------------------------------------------------------------------
	// Create — no dbID in status means this is a brand-new resource.
	// -----------------------------------------------------------------------
	if db.Status.DbID == "" {
		resp, err := api.Create(apiReq)
		if err != nil {
			log.Error(err, "failed to create database in API")
			return ctrl.Result{RequeueAfter: 30 * time.Second},
				r.setStatus(ctx, &db, "Failed", err.Error(), "")
		}
		log.Info("database created in API", "dbID", resp.ID, "phase", resp.Phase)
		if err := r.setStatus(ctx, &db, resp.Phase, resp.Message, resp.ID); err != nil {
			return ctrl.Result{}, err
		}
		if resp.Phase == "Creating" {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	// -----------------------------------------------------------------------
	// Update — dbID exists. Syncs spec changes and fetches latest phase.
	// If the API returns 404 the record was lost (e.g. DB wiped); clear the
	// dbID so the next reconcile falls through to Create.
	// -----------------------------------------------------------------------
	resp, err := api.Update(db.Status.DbID, apiReq)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			log.Info("database not found in API, re-creating", "dbID", db.Status.DbID)
			return ctrl.Result{}, r.setStatus(ctx, &db, "Pending", "Record lost — re-creating", "")
		}
		log.Error(err, "failed to update database in API", "dbID", db.Status.DbID)
		return ctrl.Result{RequeueAfter: 30 * time.Second},
			r.setStatus(ctx, &db, "Failed", err.Error(), db.Status.DbID)
	}
	log.Info("database synced with API", "dbID", resp.ID, "phase", resp.Phase)
	if err := r.setStatus(ctx, &db, resp.Phase, resp.Message, resp.ID); err != nil {
		return ctrl.Result{}, err
	}

	// -----------------------------------------------------------------------
	// Suspended: user set spec.suspended=true via kubectl.
	// Stop the database if not already stopped; suppress self-healing.
	// -----------------------------------------------------------------------
	if db.Spec.Suspended {
		if resp.Phase != "Stopped" {
			log.Info("database suspended via spec, stopping", "dbID", resp.ID)
			if err := api.UpdateStatus(resp.ID, "Stopped", "Suspended via kubectl"); err != nil {
				log.Error(err, "failed to suspend database", "dbID", resp.ID)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		}
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	switch resp.Phase {
	case "Creating", "Starting":
		// Provisioning in progress — poll every 10s until Ready.
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	case "Stopped":
		// Self-healing: operator kicks the database back into Starting.
		log.Info("database is Stopped, restarting via API", "dbID", resp.ID)
		if err := api.UpdateStatus(resp.ID, "Starting", "Self-healing: restarted by operator"); err != nil {
			log.Error(err, "failed to restart stopped database", "dbID", resp.ID)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	case "Ready":
		// Periodic check to catch external Stopped transitions.
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	default:
		return ctrl.Result{}, nil
	}
}

func (r *OracleDatabaseReconciler) setStatus(ctx context.Context, db *oraclev1alpha1.OracleDatabase, phase, message, dbID string) error {
	db.Status.Phase = phase
	db.Status.Message = message
	db.Status.DbID = dbID // empty string intentionally clears a stale ID
	return r.Status().Update(ctx, db)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OracleDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&oraclev1alpha1.OracleDatabase{}).
		Named("oracledatabase").
		Complete(r)
}
