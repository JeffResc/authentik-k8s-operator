package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OAuth2ProviderSpec defines the OAuth2 provider configuration
type OAuth2ProviderSpec struct {
	// AuthorizationFlow is the flow used for authorization
	// +kubebuilder:validation:Required
	AuthorizationFlow string `json:"authorizationFlow"`

	// RedirectURIs is the list of allowed redirect URIs
	// +kubebuilder:validation:MinItems=1
	RedirectURIs []string `json:"redirectUris"`

	// Scopes is deprecated and will be removed in a future version.
	// Use propertyMappings instead to configure OAuth2 scope mappings.
	// This field is currently ignored.
	// +kubebuilder:default={"openid","profile","email"}
	// +optional
	Scopes []string `json:"scopes,omitempty"`

	// ClientType is the OAuth2 client type (confidential or public)
	// +kubebuilder:validation:Enum=confidential;public
	// +kubebuilder:default=confidential
	// +optional
	ClientType string `json:"clientType,omitempty"`

	// AccessCodeValidity defines how long access codes are valid
	// +kubebuilder:default="minutes=1"
	// +optional
	AccessCodeValidity string `json:"accessCodeValidity,omitempty"`

	// AccessTokenValidity defines how long access tokens are valid
	// +kubebuilder:default="minutes=5"
	// +optional
	AccessTokenValidity string `json:"accessTokenValidity,omitempty"`

	// RefreshTokenValidity defines how long refresh tokens are valid
	// +kubebuilder:default="days=30"
	// +optional
	RefreshTokenValidity string `json:"refreshTokenValidity,omitempty"`

	// SubMode configures what data is included in the subject claim
	// +kubebuilder:validation:Enum=hashed_user_id;user_id;user_uuid;user_username;user_email;user_upn
	// +kubebuilder:default=hashed_user_id
	// +optional
	SubMode string `json:"subMode,omitempty"`

	// IncludeClaimsInIDToken includes claims in the ID token
	// +kubebuilder:default=true
	// +optional
	IncludeClaimsInIDToken *bool `json:"includeClaimsInIdToken,omitempty"`

	// IssuerMode configures how the issuer is determined
	// +kubebuilder:validation:Enum=global;per_provider
	// +kubebuilder:default=per_provider
	// +optional
	IssuerMode string `json:"issuerMode,omitempty"`

	// PropertyMappings is a list of property mapping UUIDs to apply
	// +optional
	PropertyMappings []string `json:"propertyMappings,omitempty"`
}

// SecretSpec defines the output secret configuration
type SecretSpec struct {
	// Name is the name of the secret to create
	// Defaults to {CR name}-oauth
	// +optional
	Name string `json:"name,omitempty"`

	// Labels to add to the secret
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations to add to the secret
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Template is a Go template for custom secret data
	// Available variables: .ClientID, .ClientSecret, .IssuerURL, .AuthURL, .TokenURL, .UserInfoURL, .Slug, .Name
	// If not specified, a default OAuth2 template is used
	// +optional
	Template string `json:"template,omitempty"`
}

// AuthentikApplicationSpec defines the desired state of AuthentikApplication
type AuthentikApplicationSpec struct {
	// Name is the display name of the application in Authentik
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Slug is the URL-friendly identifier for the application
	// Defaults to metadata.name if not specified
	// +kubebuilder:validation:Pattern=`^[a-z0-9-]+$`
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
	// +optional
	MetaLaunchURL string `json:"metaLaunchUrl,omitempty"`

	// MetaDescription is the application description
	// +optional
	MetaDescription string `json:"metaDescription,omitempty"`

	// MetaPublisher is the application publisher
	// +optional
	MetaPublisher string `json:"metaPublisher,omitempty"`

	// OpenInNewTab opens the application in a new browser tab
	// +kubebuilder:default=true
	// +optional
	OpenInNewTab *bool `json:"openInNewTab,omitempty"`

	// Provider configures the OAuth2 provider settings
	// +kubebuilder:validation:Required
	Provider OAuth2ProviderSpec `json:"provider"`

	// Secret configures the output Kubernetes secret
	// +optional
	Secret SecretSpec `json:"secret,omitempty"`
}

// AuthentikApplicationStatus defines the observed state of AuthentikApplication
type AuthentikApplicationStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ApplicationUID is the Authentik internal UID for the application
	// +optional
	ApplicationUID string `json:"applicationUid,omitempty"`

	// ProviderID is the Authentik internal ID for the OAuth2 provider
	// +optional
	ProviderID int32 `json:"providerId,omitempty"`

	// SecretName is the name of the generated Kubernetes secret
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// ClientID is the OAuth2 client ID (for reference only)
	// +optional
	ClientID string `json:"clientId,omitempty"`

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

// AuthentikApplication is the Schema for the authentikapplications API
type AuthentikApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthentikApplicationSpec   `json:"spec,omitempty"`
	Status AuthentikApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AuthentikApplicationList contains a list of AuthentikApplication
type AuthentikApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthentikApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthentikApplication{}, &AuthentikApplicationList{})
}

// GetSlug returns the slug, defaulting to metadata.name if not specified
func (a *AuthentikApplication) GetSlug() string {
	if a.Spec.Slug != "" {
		return a.Spec.Slug
	}
	return a.Name
}

// GetSecretName returns the secret name, defaulting to {name}-oauth if not specified
func (a *AuthentikApplication) GetSecretName() string {
	if a.Spec.Secret.Name != "" {
		return a.Spec.Secret.Name
	}
	return a.Name + "-oauth"
}

// GetProviderName returns a consistent name for the OAuth2 provider
func (a *AuthentikApplication) GetProviderName() string {
	return a.GetSlug() + "-provider"
}

// Condition types
const (
	// ConditionTypeReady indicates the resource has been successfully reconciled
	ConditionTypeReady = "Ready"
	// ConditionTypeSynced indicates the resource has been synced to Authentik
	ConditionTypeSynced = "Synced"
)

// Condition reasons
const (
	ReasonSucceeded        = "Succeeded"
	ReasonFailed           = "Failed"
	ReasonInProgress       = "InProgress"
	ReasonAuthentikError   = "AuthentikError"
	ReasonSecretError      = "SecretError"
	ReasonTemplateError    = "TemplateError"
	ReasonValidationFailed = "ValidationFailed"
)
