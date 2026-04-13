package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SAMLProviderSpec defines the SAML provider configuration
type SAMLProviderSpec struct {
	// AuthorizationFlow is the flow used for authorization
	// +kubebuilder:validation:Required
	AuthorizationFlow string `json:"authorizationFlow"`

	// InvalidationFlow is the flow used for session invalidation/logout
	// +kubebuilder:validation:Required
	InvalidationFlow string `json:"invalidationFlow"`

	// ACSUrl is the Assertion Consumer Service URL where Authentik sends SAML responses
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ACSUrl string `json:"acsUrl"`

	// Issuer is the EntityID of the IdP. When left empty, defaults to the Authentik base URL.
	// +optional
	Issuer string `json:"issuer,omitempty"`

	// Audience is the value of the audience restriction field.
	// When left empty, no audience restriction will be added.
	// +optional
	Audience string `json:"audience,omitempty"`

	// SPBinding configures how the SAML response is delivered to the SP
	// +kubebuilder:validation:Enum=redirect;post
	// +kubebuilder:default=redirect
	// +optional
	SPBinding string `json:"spBinding,omitempty"`

	// SigningKeypair is the name of the certificate keypair used to sign responses
	// +optional
	SigningKeypair string `json:"signingKeypair,omitempty"`

	// DigestAlgorithm is the algorithm used to compute the digest
	// +kubebuilder:validation:Enum="http://www.w3.org/2001/04/xmlenc#sha256";"http://www.w3.org/2000/09/xmldsig#sha1"
	// +kubebuilder:default="http://www.w3.org/2001/04/xmlenc#sha256"
	// +optional
	DigestAlgorithm string `json:"digestAlgorithm,omitempty"`

	// SignatureAlgorithm is the algorithm used to sign SAML responses
	// +kubebuilder:validation:Enum="http://www.w3.org/2001/04/xmldsig-more#rsa-sha256";"http://www.w3.org/2000/09/xmldsig#rsa-sha1";"http://www.w3.org/2000/09/xmldsig#dsa-sha1"
	// +kubebuilder:default="http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
	// +optional
	SignatureAlgorithm string `json:"signatureAlgorithm,omitempty"`

	// PropertyMappings is a list of property mapping UUIDs to apply
	// +optional
	PropertyMappings []string `json:"propertyMappings,omitempty"`
}

// AuthentikSAMLApplicationSpec defines the desired state of AuthentikSAMLApplication
type AuthentikSAMLApplicationSpec struct {
	// Name is the display name of the application in Authentik
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Name string `json:"name"`

	// Slug is the URL-friendly identifier for the application
	// Defaults to metadata.name if not specified
	// +kubebuilder:validation:Pattern=`^[a-z0-9-]+$`
	// +kubebuilder:validation:MaxLength=128
	// +optional
	Slug string `json:"slug,omitempty"`

	// Group is the application group in Authentik
	// +optional
	Group string `json:"group,omitempty"`

	// PolicyEngineMode determines how policies are evaluated
	// +kubebuilder:validation:Enum=all;any
	// +kubebuilder:default=any
	// +optional
	PolicyEngineMode string `json:"policyEngineMode,omitempty"`

	// MetaLaunchURL is the URL to launch the application
	// +kubebuilder:validation:MaxLength=2048
	// +optional
	MetaLaunchURL string `json:"metaLaunchUrl,omitempty"`

	// MetaDescription is the application description
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	MetaDescription string `json:"metaDescription,omitempty"`

	// MetaPublisher is the application publisher
	// +kubebuilder:validation:MaxLength=256
	// +optional
	MetaPublisher string `json:"metaPublisher,omitempty"`

	// OpenInNewTab opens the application in a new browser tab
	// +kubebuilder:default=true
	// +optional
	OpenInNewTab *bool `json:"openInNewTab,omitempty"`

	// Provider configures the SAML provider settings
	// +kubebuilder:validation:Required
	Provider SAMLProviderSpec `json:"provider"`

	// Secret configures the output Kubernetes secret containing SAML metadata
	// +optional
	Secret SecretSpec `json:"secret,omitempty"`
}

// AuthentikSAMLApplicationStatus defines the observed state of AuthentikSAMLApplication
type AuthentikSAMLApplicationStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ApplicationUID is the Authentik internal UID for the application
	// +optional
	ApplicationUID string `json:"applicationUid,omitempty"`

	// ProviderID is the Authentik internal ID for the SAML provider
	// +optional
	ProviderID int32 `json:"providerId,omitempty"`

	// SecretName is the name of the generated Kubernetes secret
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// ObservedGeneration is the last observed generation of the resource
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Application UID",type="string",JSONPath=".status.applicationUid"
// +kubebuilder:printcolumn:name="Provider ID",type="integer",JSONPath=".status.providerId"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AuthentikSAMLApplication is the Schema for the authentiksamlapplications API
type AuthentikSAMLApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthentikSAMLApplicationSpec   `json:"spec,omitempty"`
	Status AuthentikSAMLApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AuthentikSAMLApplicationList contains a list of AuthentikSAMLApplication
type AuthentikSAMLApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthentikSAMLApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthentikSAMLApplication{}, &AuthentikSAMLApplicationList{})
}

// GetSlug returns the slug, defaulting to metadata.name if not specified
func (a *AuthentikSAMLApplication) GetSlug() string {
	if a.Spec.Slug != "" {
		return a.Spec.Slug
	}
	return a.Name
}

// GetSecretName returns the secret name, defaulting to {name}-saml
func (a *AuthentikSAMLApplication) GetSecretName() string {
	if a.Spec.Secret.Name != "" {
		return a.Spec.Secret.Name
	}
	return a.Name + "-saml"
}

// GetProviderName returns a consistent name for the SAML provider
func (a *AuthentikSAMLApplication) GetProviderName() string {
	return a.GetSlug() + "-provider"
}
