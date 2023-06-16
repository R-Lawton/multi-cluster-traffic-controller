package dnsprovider

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/Kuadrant/multicluster-gateway-controller/pkg/apis/v1alpha1"
	"github.com/Kuadrant/multicluster-gateway-controller/pkg/dns"
	"github.com/Kuadrant/multicluster-gateway-controller/pkg/dns/aws"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var errUnsupportedProvider = fmt.Errorf("provider type given is not supported")

type providerFactory struct {
	client client.Client
}

func NewProvider(c client.Client) *providerFactory {

	return &providerFactory{
		client: c,
	}
}

func (p *providerFactory) loadProviderSecret(ctx context.Context, managedZone *v1alpha1.ManagedZone) (*v1.Secret, *v1alpha1.DNSProvider, error) {
	dnsProvider := &v1alpha1.DNSProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedZone.Spec.ProviderRef.Name,
			Namespace: managedZone.Spec.ProviderRef.Namespace,
		}}
	fmt.Print("I get here4")

	log.Log.Info("Reconciling DNS Provider:", "Name:", dnsProvider.Name)
	err := p.client.Get(ctx, client.ObjectKeyFromObject(dnsProvider), dnsProvider)
	if err != nil {
		return nil, nil, err
	}
	fmt.Print("I get here5")

	providerSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dnsProvider.Spec.Credentials.Name,
			Namespace: dnsProvider.Spec.Credentials.Namespace,
		}}

	if err := p.client.Get(ctx, client.ObjectKeyFromObject(providerSecret), providerSecret); err != nil {

		return nil, nil, err

	}

	return providerSecret, dnsProvider, nil
}

func (p *providerFactory) DNSProviderFactory(ctx context.Context, managedZone *v1alpha1.ManagedZone) (dns.Provider, error) {
	// fmt.Print("I get here3")

	creds, provider, err := p.loadProviderSecret(ctx, managedZone)
	if err != nil {
		return nil, fmt.Errorf("unable to load dns provider secret: %v", err)
	}
	var DNSProvider dns.Provider

	switch strings.ToUpper(provider.Spec.Credentials.ProviderType) {
	case "AWS":
		log.Log.Info("Creating DNS provider for provider type AWS")
		DNSProvider, err := aws.NewProviderFromSecret(creds)
		if err != nil {

			return nil, fmt.Errorf("unable to create dns provider from secret: %v", err)

		}
		return DNSProvider, nil

	case "GCP":
		log.Log.Info("GCP")

	case "AZURE":
		log.Log.Info("AZURE")

	default:
		return nil, errUnsupportedProvider
	}

	return DNSProvider, nil
}
