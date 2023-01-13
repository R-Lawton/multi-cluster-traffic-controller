package traffic

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/strings/slices"

	"github.com/Kuadrant/multi-cluster-traffic-controller/pkg/_internal/slice"
)

func NewIngress(i *networkingv1.Ingress) *Ingress {
	return &Ingress{Ingress: i}
}

type Ingress struct {
	*networkingv1.Ingress
	generatedHost string
}

func (a *Ingress) GetKind() string {
	return "Ingress"
}

func (a *Ingress) GetHosts() []string {
	var hosts []string
	for _, rule := range a.Spec.Rules {
		if !slices.Contains(hosts, rule.Host) {
			hosts = append(hosts, rule.Host)
		}
	}

	return hosts
}

func (a *Ingress) AddManagedHost(h string) error {
	// rules to add to the spec
	additionalRules := []networkingv1.IngressRule{}
	// rules we have covered already in the spec
	coveredRules := []*networkingv1.HTTPIngressRuleValue{}
	for _, existing := range a.Spec.Rules {
		if existing.Host == h {
			coveredRules = append(coveredRules, existing.HTTP)
		}
	}

	var isCovered = func(val *networkingv1.HTTPIngressRuleValue) bool {
		for _, ar := range coveredRules {
			if equality.Semantic.DeepEqual(ar, val) {
				return true
			}
		}
		return false
	}
	// we now know what rules we have already covered so now calculate any new ones
	for _, existing := range a.Spec.Rules {
		if existing.Host == h || isCovered(existing.HTTP) {
			continue
		}

		additional := existing.DeepCopy()
		additional.Host = h
		additionalRules = append(additionalRules, *additional)
		coveredRules = append(coveredRules, additional.HTTP)
	}
	a.Spec.Rules = append(a.Spec.Rules, additionalRules...)
	return nil
}

func (a *Ingress) AddTLS(host string, secret *corev1.Secret) {
	for i, tls := range a.Spec.TLS {
		if slice.ContainsString(tls.Hosts, host) {
			a.Spec.TLS[i] = networkingv1.IngressTLS{
				Hosts:      []string{host},
				SecretName: secret.Name,
			}
			return
		}
	}
	a.Spec.TLS = append(a.Spec.TLS, networkingv1.IngressTLS{
		Hosts:      []string{host},
		SecretName: secret.GetName(),
	})
}

func (a *Ingress) RemoveTLS(hosts []string) {
	for _, removeHost := range hosts {
		for i, tls := range a.Spec.TLS {
			tlsHosts := tls.Hosts
			for j, host := range tls.Hosts {
				if host == removeHost {
					tlsHosts = append(hosts[:j], hosts[j+1:]...)
				}
			}
			// if there are no hosts remaining remove the entry for TLS
			if len(tlsHosts) == 0 {
				a.Spec.TLS = append(a.Spec.TLS[:i], a.Spec.TLS[i+1:]...)
			}
		}
	}
}

func (a *Ingress) GetSpec() interface{} {
	return a.Spec
}

func (a *Ingress) GetNamespaceName() types.NamespacedName {
	return types.NamespacedName{
		Namespace: a.Namespace,
		Name:      a.Name,
	}
}

func (a *Ingress) GetCacheKey() string {
	key, _ := cache.MetaNamespaceKeyFunc(a)
	return key
}

func (a *Ingress) String() string {
	return fmt.Sprintf("kind: %v, namespace/name: %v", a.GetKind(), a.GetNamespaceName())
}
