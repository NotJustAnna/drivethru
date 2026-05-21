// Package v1alpha1 contains API Schema definitions for the drivethru.notjustanna.net
// v1alpha1 API group.
//
// +kubebuilder:object:generate=true
// +groupName=drivethru.notjustanna.net
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion = schema.GroupVersion{Group: "drivethru.notjustanna.net", Version: "v1alpha1"}

	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	AddToScheme = SchemeBuilder.AddToScheme
)
