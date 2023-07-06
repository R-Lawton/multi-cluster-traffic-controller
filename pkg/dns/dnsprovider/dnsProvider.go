package dnsprovider

import (
	"context"
	"fmt"
	// "regexp"

	// "strings"

	// "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/Kuadrant/multicluster-gateway-controller/pkg/_internal/conditions"
	"github.com/Kuadrant/multicluster-gateway-controller/pkg/apis/v1alpha1"
	"github.com/Kuadrant/multicluster-gateway-controller/pkg/controllers/managedzone"
	"github.com/Kuadrant/multicluster-gateway-controller/pkg/dns"
	"github.com/Kuadrant/multicluster-gateway-controller/pkg/dns/aws"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var errUnsupportedProvider = fmt.Errorf("provider type given is not supported")

type providerFactory struct {
	client.Client
}

func NewProvider(c client.Client) *providerFactory {

	return &providerFactory{
		Client: c,
	}
}

// depending on the provider type given in dnsprovider cr using the credentials supplied returns a dnsprovider.
func (p *providerFactory) DNSProviderFactory(ctx context.Context, managedZone *v1alpha1.ManagedZone) (dns.Provider, error) {

	var reason, message string
	status := metav1.ConditionTrue
	reason = "ProviderSuccess"
	message = "Provider was created successfully from secret"
	providerSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedZone.Spec.SecretRef.Name,
			Namespace: managedZone.Spec.SecretRef.Namespace,
		}}

	if err := p.Client.Get(ctx, client.ObjectKeyFromObject(providerSecret), providerSecret); err != nil {

		return nil, err
	}
	// providertype, err := regexp.MatchString("kuadrant.io/",string(providerSecret.Type))
	// if err != nil{
	// 	return nil, err
	// }
	// if providerSecret.Type != providertype{
	// 	return fmt.Errorf("")

	// }

	switch providerSecret.Type {
	case "kuadrant.io/aws":
		log.Log.Info("Creating DNS provider for provider type AWS")
		DNSProvider, err := aws.NewProviderFromSecret(providerSecret)
		if err != nil {
			status = metav1.ConditionFalse
			reason = "DNSProviderErrorState"
			message = "DNS Provider could not be created from secret"

			managedzone.SetManagedZoneCondition(managedZone, conditions.ConditionTypeReady, status, reason, message)
			err = p.Status().Update(ctx, managedZone)
			if err != nil {
				return nil, err
			}

			return nil, fmt.Errorf("unable to create dns provider from secret: %v", err)
		}
		log.Log.Info("Route53 provider created")

		return DNSProvider, nil

	default:
		return nil, errUnsupportedProvider
	}

}
