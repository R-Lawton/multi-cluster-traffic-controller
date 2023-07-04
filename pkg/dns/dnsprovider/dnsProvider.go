package dnsprovider

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/Kuadrant/multicluster-gateway-controller/pkg/_internal/conditions"
	"github.com/Kuadrant/multicluster-gateway-controller/pkg/apis/v1alpha1"
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

func (p *providerFactory) loadProviderSecret(ctx context.Context, managedZone *v1alpha1.ManagedZone) (*v1.Secret, *v1alpha1.DNSProvider, error) {

	dnsProvider := &v1alpha1.DNSProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedZone.Spec.ProviderRef.Name,
			Namespace: managedZone.Spec.ProviderRef.Namespace,
		}}

	err := p.Client.Get(ctx, client.ObjectKeyFromObject(dnsProvider), dnsProvider)
	if err != nil {

		return nil, nil, err
	}
	log.Log.Info("DNS Provider CR found:", "Name:", dnsProvider.Name)

	providerSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dnsProvider.Spec.Credentials.Name,
			Namespace: dnsProvider.Spec.Credentials.Namespace,
		}}

	if err := p.Client.Get(ctx, client.ObjectKeyFromObject(providerSecret), providerSecret); err != nil {

		return nil, nil, err

	}

	return providerSecret, dnsProvider, nil
}

func (p *providerFactory) DNSProviderFactory(ctx context.Context, managedZone *v1alpha1.ManagedZone) (dns.Provider, error) {
	var reason, message string
	status := metav1.ConditionTrue
	reason = "ProviderSuccess"
	message = "Provider was created successfully from secret"

	creds, provider, err := p.loadProviderSecret(ctx, managedZone)
	if err != nil {
		status = metav1.ConditionFalse
		reason = "DNSProviderCredentialError"
		message = "DNS Provider credentials could not be loaded from secret"

		setDnsProviderCondition(provider, conditions.ConditionTypeReady, status, reason, message)
		err = p.Status().Update(ctx, provider)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unable to load dns provider secret: %v", err)
	}

	switch strings.ToUpper(provider.Spec.Credentials.ProviderType) {
	case "AWS":
		log.Log.Info("Creating DNS provider for provider type AWS")
		DNSProvider, err := aws.NewProviderFromSecret(creds)
		if err != nil {
			status = metav1.ConditionFalse
			reason = "DNSProviderErrorState"
			message = "DNS Provider could not be created from secret"

			setDnsProviderCondition(provider, conditions.ConditionTypeReady, status, reason, message)
			err = p.Status().Update(ctx, provider)
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("unable to create dns provider from secret: %v", err)
		}
		log.Log.Info("Route53 provider created")

		setDnsProviderCondition(provider, conditions.ConditionTypeReady, status, reason, message)
		err = p.Status().Update(ctx, provider)
		if err != nil {
			return nil, err
		}

		return DNSProvider, nil

	case "GCP":
		log.Log.Info("GCP")

	case "AZURE":
		log.Log.Info("AZURE")

	default:
		return nil, errUnsupportedProvider
	}

	return nil, nil
}

func setDnsProviderCondition(provider *v1alpha1.DNSProvider, conditionType string, status metav1.ConditionStatus, reason, message string) {
	cond := metav1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
	meta.SetStatusCondition(&provider.Status.Conditions, cond)
}
