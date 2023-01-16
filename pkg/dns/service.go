package dns

import (
	"context"
	"fmt"
	"os"
	"strings"

	v1 "github.com/Kuadrant/multi-cluster-traffic-controller/pkg/apis/v1"
	"github.com/Kuadrant/multi-cluster-traffic-controller/pkg/traffic"
	"github.com/lithammer/shortuuid/v4"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Service struct {
	controlClient client.Client
	// this is temporary setting the tenant ns in the control plane.
	// will be removed when we have auth that can map to a given ctrl plane ns
	defaultCtrlNS string
}

func NewService(controlClient client.Client, defaultCtrlNS string) *Service {
	return &Service{controlClient: controlClient, defaultCtrlNS: defaultCtrlNS}
}

func (s *Service) EnsureManagedHost(ctx context.Context, t traffic.Interface) (*v1.DNSRecord, error) {
	dnsRecord, err := s.getManagedRecord(ctx, t)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}
	if dnsRecord != nil {
		fmt.Println("found host ", dnsRecord.Name)
		return dnsRecord, nil
	}
	fmt.Println("no managed host generating one")
	hostSubdomain := shortuuid.NewWithNamespace(t.GetNamespace() + t.GetName())
	zones := getManagedZones()
	var chosenZone zone
	var managedHost string
	for _, z := range zones {
		if z.Default {
			managedHost = strings.ToLower(fmt.Sprintf("%s.%s", hostSubdomain, z.RootDomain))
			chosenZone = z
			break
		}
	}
	if chosenZone.ID == "" {
		return nil, fmt.Errorf("no zone available to use")
	}
	record, err := s.RegisterHost(ctx, managedHost, chosenZone.DNSZone)
	if err != nil {
		fmt.Println("failed to register host ", err)
		return nil, err
	}
	return record, nil
}

func (s *Service) RegisterHost(ctx context.Context, h string, zone v1.DNSZone) (*v1.DNSRecord, error) {
	fmt.Println("registering host ", h)
	dnsRecord := v1.DNSRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h,
			Namespace: s.defaultCtrlNS,
		},
	}

	err := s.controlClient.Create(ctx, &dnsRecord, &client.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, err
	}
	//host may already be present
	if err != nil && k8serrors.IsAlreadyExists(err) {
		err = s.controlClient.Get(ctx, client.ObjectKeyFromObject(&dnsRecord), &dnsRecord)
		if err != nil {
			return nil, err
		}
	}
	return &dnsRecord, nil
}

func (s *Service) getManagedRecord(ctx context.Context, t traffic.Interface) (*v1.DNSRecord, error) {
	existing := t.GetHosts()
	zones := getManagedZones()
	var managedHost string
	var dnsRecord = v1.DNSRecord{}
	// stupid simple for now
	for _, h := range existing {
		for _, z := range zones {
			if strings.HasSuffix(h, z.RootDomain) {
				managedHost = h
				break
			}
		}
	}
	if err := s.controlClient.Get(ctx, client.ObjectKey{Namespace: s.defaultCtrlNS, Name: managedHost}, &dnsRecord); err != nil {
		return nil, err
	}
	return &dnsRecord, nil
}

// this is temporary and will be replaced in the future by CRD resources
type zone struct {
	v1.DNSZone
	RootDomain string
	Default    bool
}

// this is temporary and will be replaced in the future by CRD resources
func getManagedZones() []zone {
	return []zone{{
		DNSZone:    v1.DNSZone{ID: os.Getenv("AWS_DNS_PUBLIC_ZONE_ID")},
		RootDomain: os.Getenv("ZONE_ROOT_DOMAIN"),
		Default:    true,
	}}
}
