package v1alpha1

import (
	"bytes"
	"encoding/json"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Finalizer = "drivethru.notjustanna.net/finalizer"

	ConditionReady = "Ready"
)

// SecretNameOption is a union type: a string (custom secret name) or the JSON
// literal `false` (disable Secret generation). Unset means "use default name".
//
// +kubebuilder:validation:XPreserveUnknownFields
type SecretNameOption struct {
	Name     string `json:"-"`
	Disabled bool   `json:"-"`
}

func (s *SecretNameOption) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil
	}
	if bytes.Equal(data, []byte("false")) {
		s.Disabled = true
		return nil
	}
	if bytes.Equal(data, []byte("true")) {
		return errors.New("generatedSecretName: true is not a valid value; use a string or false")
	}
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return errors.New("generatedSecretName: must be a string or false")
	}
	s.Name = str
	return nil
}

func (s SecretNameOption) MarshalJSON() ([]byte, error) {
	if s.Disabled {
		return []byte("false"), nil
	}
	return json.Marshal(s.Name)
}

// NameOrDefault returns the secret name to use, or the default fallback.
// Callers must check Disabled() first.
func (s *SecretNameOption) NameOrDefault(fallback string) string {
	if s == nil || s.Name == "" {
		return fallback
	}
	return s.Name
}

// IsDisabled reports whether Secret generation is disabled.
func (s *SecretNameOption) IsDisabled() bool {
	return s != nil && s.Disabled
}

// DeepCopyInto copies the receiver into out.
func (s *SecretNameOption) DeepCopyInto(out *SecretNameOption) {
	*out = *s
}

// DeepCopy returns a deep copy.
func (s *SecretNameOption) DeepCopy() *SecretNameOption {
	if s == nil {
		return nil
	}
	out := new(SecretNameOption)
	s.DeepCopyInto(out)
	return out
}

// StaticSiteSpec defines the desired state of a StaticSite.
type StaticSiteSpec struct {
	// Host is the public hostname. Used as Garage bucket name, website host,
	// and Traefik Host() rule.
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// GeneratedSecretName overrides the name of the generated S3-credentials
	// Secret, or disables Secret generation when set to literal `false`.
	// Defaults to `{metadata.name}-s3` when unset.
	// +optional
	GeneratedSecretName *SecretNameOption `json:"generatedSecretName,omitempty"`

	// Retain skips bucket and credentials deletion when the resource is removed.
	// +optional
	Retain bool `json:"retain,omitempty"`
}

// StaticSiteStatus defines the observed state of a StaticSite.
type StaticSiteStatus struct {
	// Ready indicates that the bucket, credentials and IngressRoute are
	// reconciled.
	Ready bool `json:"ready,omitempty"`

	// BucketName is the resolved bucket name in Garage.
	BucketName string `json:"bucketName,omitempty"`

	// SecretName is the name of the generated S3-credentials Secret, if any.
	SecretName string `json:"secretName,omitempty"`

	// Conditions report the latest reconciliation state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ss
// +kubebuilder:printcolumn:name="Host",type=string,JSONPath=`.spec.host`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// StaticSite provisions a static website backed by a Garage bucket and exposed
// through a Traefik IngressRoute.
type StaticSite struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StaticSiteSpec   `json:"spec,omitempty"`
	Status StaticSiteStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// StaticSiteList is a list of StaticSite resources.
type StaticSiteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StaticSite `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StaticSite{}, &StaticSiteList{})
}
