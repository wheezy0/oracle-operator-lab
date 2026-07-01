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

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	oraclev1alpha1 "dboperator.io/oracle-operator/api/v1alpha1"
)

var oracledatabaselog = logf.Log.WithName("oracledatabase-webhook")

// SetupOracleDatabaseWebhookWithManager registers the webhook for OracleDatabase in the manager.
func SetupOracleDatabaseWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &oraclev1alpha1.OracleDatabase{}).
		WithValidator(&OracleDatabaseCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-oracle-dboperator-io-v1alpha1-oracledatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=oracle.dboperator.io,resources=oracledatabases,verbs=create;update,versions=v1alpha1,name=voracledatabase-v1alpha1.kb.io,admissionReviewVersions=v1

// OracleDatabaseCustomValidator validates OracleDatabase resources on create and update.
// +kubebuilder:object:generate=false
type OracleDatabaseCustomValidator struct {
	client.Client
}

// ValidateCreate checks that the dbName is not already used by another OracleDatabase in the cluster.
func (v *OracleDatabaseCustomValidator) ValidateCreate(ctx context.Context, obj *oraclev1alpha1.OracleDatabase) (admission.Warnings, error) {
	oracledatabaselog.Info("validating create", "name", obj.Name, "dbName", obj.Spec.DbName)
	return nil, v.checkDbNameUnique(ctx, obj.Spec.DbName, obj.Namespace, obj.Name)
}

// ValidateUpdate checks that if dbName is changed, the new name is not already in use.
func (v *OracleDatabaseCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *oraclev1alpha1.OracleDatabase) (admission.Warnings, error) {
	oracledatabaselog.Info("validating update", "name", newObj.Name, "dbName", newObj.Spec.DbName)
	if oldObj.Spec.DbName == newObj.Spec.DbName {
		return nil, nil
	}
	return nil, v.checkDbNameUnique(ctx, newObj.Spec.DbName, newObj.Namespace, newObj.Name)
}

// ValidateDelete — no validation needed on deletion.
func (v *OracleDatabaseCustomValidator) ValidateDelete(_ context.Context, obj *oraclev1alpha1.OracleDatabase) (admission.Warnings, error) {
	return nil, nil
}

// checkDbNameUnique lists all OracleDatabase resources across all namespaces and rejects
// the request if any existing resource (other than the one being validated) already uses the same dbName.
func (v *OracleDatabaseCustomValidator) checkDbNameUnique(ctx context.Context, dbName, namespace, name string) error {
	var list oraclev1alpha1.OracleDatabaseList
	if err := v.List(ctx, &list, &client.ListOptions{}); err != nil {
		return fmt.Errorf("failed to list OracleDatabases: %w", err)
	}

	for _, existing := range list.Items {
		if existing.Spec.DbName == dbName && !(existing.Namespace == namespace && existing.Name == name) {
			return field.Invalid(
				field.NewPath("spec").Child("dbName"),
				dbName,
				fmt.Sprintf("already in use by OracleDatabase %s/%s", existing.Namespace, existing.Name),
			)
		}
	}
	return nil
}
