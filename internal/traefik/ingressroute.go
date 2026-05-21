package traefik

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildIngressRoute constructs an IngressRoute that routes Host(host) to the
// given in-cluster Garage service.
func BuildIngressRoute(name, namespace, host, entrypoint, certResolver, svcName, svcNamespace string, svcPort int) *IngressRoute {
	tls := &TLSBlock{}
	if certResolver != "" {
		tls.CertResolver = certResolver
	}

	return &IngressRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GroupVersion.String(),
			Kind:       "IngressRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: IngressRouteSpec{
			EntryPoints: []string{entrypoint},
			Routes: []Route{{
				Match: fmt.Sprintf("Host(`%s`)", host),
				Kind:  "Rule",
				Services: []Service{{
					Name:      svcName,
					Namespace: svcNamespace,
					Port:      svcPort,
					Kind:      "Service",
				}},
			}},
			TLS: tls,
		},
	}
}
