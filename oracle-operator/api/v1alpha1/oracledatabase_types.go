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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OracleDatabaseSpec defines the desired state of OracleDatabase
type OracleDatabaseSpec struct {
	// dbName is the name of the Oracle database (SID or CDB name).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=8
	DbName string `json:"dbName"`

	// owner is the Oracle DBA username that will own this database.
	// +kubebuilder:validation:MinLength=1
	Owner string `json:"owner"`

	// version is the Oracle database version, e.g. "19c" or "21c".
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// characterSet is the Oracle character set, e.g. "AL32UTF8".
	// +kubebuilder:default=AL32UTF8
	// +optional
	CharacterSet string `json:"characterSet,omitempty"`

	// sizeGB is the initial allocated size of the database in gigabytes.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65536
	SizeGB int32 `json:"sizeGB"`

	// serviceName is the Oracle Net service name used to connect to this database.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// pdbName is the name of the Pluggable Database (PDB) inside the CDB. Optional.
	// +optional
	PdbName string `json:"pdbName,omitempty"`

	// suspended, when true, stops the database and keeps it stopped.
	// The operator will not self-heal a suspended database.
	// Set back to false (or remove the field) to restart it.
	// +optional
	Suspended bool `json:"suspended,omitempty"`
}

// OracleDatabaseStatus defines the observed state of OracleDatabase.
type OracleDatabaseStatus struct {
	// phase is the current lifecycle phase of the database: Pending, Creating, Ready, Failed, Deleting.
	// +optional
	Phase string `json:"phase,omitempty"`

	// dbID is the ID assigned by the mock API backend once the database is created.
	// +optional
	DbID string `json:"dbID,omitempty"`

	// message is a human-readable description of the current status or last error.
	// +optional
	Message string `json:"message,omitempty"`

	// conditions represent the current state of the OracleDatabase resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="DBName",type=string,JSONPath=`.spec.dbName`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="SizeGB",type=integer,JSONPath=`.spec.sizeGB`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// OracleDatabase is the Schema for the oracledatabases API
type OracleDatabase struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OracleDatabase
	// +required
	Spec OracleDatabaseSpec `json:"spec"`

	// status defines the observed state of OracleDatabase
	// +optional
	Status OracleDatabaseStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OracleDatabaseList contains a list of OracleDatabase
type OracleDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OracleDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &OracleDatabase{}, &OracleDatabaseList{})
		return nil
	})
}
