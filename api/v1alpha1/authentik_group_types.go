package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deletion policies for AuthentikGroup
const (
	// DeletionPolicyDelete deletes the group from Authentik when the CR is deleted
	DeletionPolicyDelete = "Delete"
	// DeletionPolicyRetain keeps the group in Authentik when the CR is deleted
	DeletionPolicyRetain = "Retain"
)

// AuthentikGroupSpec defines the desired state of AuthentikGroup
type AuthentikGroupSpec struct {
	// Name is the group name in Authentik. Defaults to metadata.name.
	// +optional
	Name string `json:"name,omitempty"`

	// IsSuperuser makes users in this group superusers
	// +optional
	IsSuperuser *bool `json:"isSuperuser,omitempty"`

	// Parents is a list of parent group names (or UUIDs) in Authentik
	// +optional
	Parents []string `json:"parents,omitempty"`

	// Roles is a list of Authentik RBAC role names (or UUIDs) assigned to this group
	// +optional
	Roles []string `json:"roles,omitempty"`

	// Attributes is a free-form JSON object stored on the group
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	// +optional
	Attributes *apiextensionsv1.JSON `json:"attributes,omitempty"`

	// DeletionPolicy controls whether the group is deleted from Authentik when the CR is deleted
	// +kubebuilder:validation:Enum=Delete;Retain
	// +kubebuilder:default=Delete
	// +optional
	DeletionPolicy string `json:"deletionPolicy,omitempty"`
}

// AuthentikGroupStatus defines the observed state of AuthentikGroup
type AuthentikGroupStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// GroupUID is the Authentik internal UUID for the group
	// +optional
	GroupUID string `json:"groupUid,omitempty"`

	// ObservedGeneration is the last observed generation of the resource
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Group UID",type="string",JSONPath=".status.groupUid"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AuthentikGroup is the Schema for the authentikgroups API
type AuthentikGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthentikGroupSpec   `json:"spec,omitempty"`
	Status AuthentikGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AuthentikGroupList contains a list of AuthentikGroup
type AuthentikGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthentikGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthentikGroup{}, &AuthentikGroupList{})
}

// GetGroupName returns the group name, defaulting to metadata.name if not specified
func (g *AuthentikGroup) GetGroupName() string {
	if g.Spec.Name != "" {
		return g.Spec.Name
	}
	return g.Name
}
