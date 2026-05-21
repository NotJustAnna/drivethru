// Package traefik provides a minimal client-go-compatible representation of
// the Traefik IngressRoute CRD. We define our own types rather than pulling in
// the entire traefik/traefik module — the surface we use is tiny.
package traefik

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

const (
	GroupName = "traefik.io"
	Version   = "v1alpha1"
)

var (
	GroupVersion  = schema.GroupVersion{Group: GroupName, Version: Version}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

// IngressRoute is a (subset of the) Traefik IngressRoute CRD.
type IngressRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              IngressRouteSpec `json:"spec,omitempty"`
}

// IngressRouteSpec is the spec portion.
type IngressRouteSpec struct {
	EntryPoints []string  `json:"entryPoints,omitempty"`
	Routes      []Route   `json:"routes,omitempty"`
	TLS         *TLSBlock `json:"tls,omitempty"`
}

// Route is a single routing rule.
type Route struct {
	Match    string    `json:"match"`
	Kind     string    `json:"kind"`
	Services []Service `json:"services,omitempty"`
}

// Service is the backend service for a route.
type Service struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Port      int    `json:"port"`
	Kind      string `json:"kind,omitempty"`
	Scheme    string `json:"scheme,omitempty"`
}

// TLSBlock controls TLS termination.
type TLSBlock struct {
	CertResolver string `json:"certResolver,omitempty"`
}

// IngressRouteList is a list of IngressRoutes.
type IngressRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IngressRoute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IngressRoute{}, &IngressRouteList{})
}

// --- DeepCopy / runtime.Object plumbing -------------------------------------

func (in *IngressRoute) DeepCopyInto(out *IngressRoute) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

func (in *IngressRoute) DeepCopy() *IngressRoute {
	if in == nil {
		return nil
	}
	out := new(IngressRoute)
	in.DeepCopyInto(out)
	return out
}

func (in *IngressRoute) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *IngressRouteList) DeepCopyInto(out *IngressRouteList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]IngressRoute, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *IngressRouteList) DeepCopy() *IngressRouteList {
	if in == nil {
		return nil
	}
	out := new(IngressRouteList)
	in.DeepCopyInto(out)
	return out
}

func (in *IngressRouteList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *IngressRouteSpec) DeepCopyInto(out *IngressRouteSpec) {
	*out = *in
	if in.EntryPoints != nil {
		out.EntryPoints = append([]string(nil), in.EntryPoints...)
	}
	if in.Routes != nil {
		out.Routes = make([]Route, len(in.Routes))
		for i := range in.Routes {
			in.Routes[i].DeepCopyInto(&out.Routes[i])
		}
	}
	if in.TLS != nil {
		out.TLS = &TLSBlock{CertResolver: in.TLS.CertResolver}
	}
}

func (in *Route) DeepCopyInto(out *Route) {
	*out = *in
	if in.Services != nil {
		out.Services = append([]Service(nil), in.Services...)
	}
}
