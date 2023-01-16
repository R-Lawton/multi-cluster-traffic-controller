/*
Copyright 2022 The MultiCluster Traffic Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package traffic

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kuadrantv1 "github.com/Kuadrant/multi-cluster-traffic-controller/pkg/apis/v1"
	"github.com/Kuadrant/multi-cluster-traffic-controller/pkg/dns"
	"github.com/Kuadrant/multi-cluster-traffic-controller/pkg/dns/aws"
	"github.com/Kuadrant/multi-cluster-traffic-controller/pkg/traffic"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Reconciler reconciles a traffic object
type Reconciler struct {
	WorkloadClient client.Client
	ControlClient  client.Client
	Hosts          HostService
	Certificates   CertificateService
	HostResolver   dns.HostResolver
}

type HostService interface {
	EnsureManagedHost(ctx context.Context, t traffic.Interface) (*kuadrantv1.DNSRecord, error)
}

type CertificateService interface {
	EnsureCertificate(ctx context.Context, host string, owner metav1.Object) error
	GetCertificateSecret(ctx context.Context, host string) (*v1.Secret, error)
}

func (r *Reconciler) Handle(ctx context.Context, o runtime.Object) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	trafficAccessor := o.(traffic.Interface)
	log.Log.Info("got traffic object", "kind", trafficAccessor.GetKind(), "name", trafficAccessor.GetName(), "namespace", trafficAccessor.GetNamespace())
	// verify host is correct
	// no managed host assigned assign one
	// create empty DNSRecord with assigned host
	managedHostRecord, err := r.Hosts.EnsureManagedHost(ctx, trafficAccessor)
	if err != nil {
		return ctrl.Result{}, err
	}
	fmt.Println("managed record ", managedHostRecord)
	if err := trafficAccessor.AddManagedHost(managedHostRecord.Name); err != nil {
		return ctrl.Result{}, err
	}
	// create certificate resource for assigned host
	fmt.Println("host assigned ensuring certificate in place")
	if err := r.Certificates.EnsureCertificate(ctx, managedHostRecord.Name, managedHostRecord); err != nil && !k8serrors.IsAlreadyExists(err) {
		return ctrl.Result{}, err
	}
	// when certificate ready copy secret (need to add event handler for certs)
	// only once certificate is ready update DNS based status of ingress
	secret, err := r.Certificates.GetCertificateSecret(ctx, managedHostRecord.Name)
	if err != nil && !k8serrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	// if err is not exists return and wait
	if err != nil {
		fmt.Println("secret does not exist yet requeue")
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
	}

	//copy secret
	if secret != nil {
		fmt.Println("secret ready. copy secret", secret.Name)
		trafficAccessor.AddTLS(managedHostRecord.Name, secret)
		copySecret := secret.DeepCopy()
		copySecret.ObjectMeta = metav1.ObjectMeta{
			Name:      managedHostRecord.Name,
			Namespace: trafficAccessor.GetNamespace(),
		}
		if err := r.WorkloadClient.Create(ctx, copySecret, &client.CreateOptions{}); err != nil {
			if k8serrors.IsAlreadyExists(err) {
				if err := r.WorkloadClient.Get(ctx, client.ObjectKeyFromObject(copySecret), copySecret); err != nil {
					return ctrl.Result{}, err
				}
				copySecret.Data = secret.Data
				if err := r.WorkloadClient.Update(ctx, copySecret, &client.UpdateOptions{}); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}
	activeDNSTargetIPs := map[string][]string{}
	targets, err := trafficAccessor.GetDNSTargets()
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, target := range targets {
		host := target.Value
		if target.TargetType == kuadrantv1.TargetTypeIP {
			activeDNSTargetIPs[host] = append(activeDNSTargetIPs[host], host)
			continue
		}
		addr, err := r.HostResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("DNSLookup failed for host %s : %s", host, err)
		}
		for _, add := range addr {
			activeDNSTargetIPs[host] = append(activeDNSTargetIPs[host], add.IP.String())
		}
	}
	//TODO we need to watch certs / secrets and trigger reconcle
	copyDNS := managedHostRecord.DeepCopy()
	r.setEndpointFromTargets(managedHostRecord.Name, activeDNSTargetIPs, copyDNS)
	fmt.Println("updated dNSRecordn ", copyDNS.Spec)
	if err := r.WorkloadClient.Update(ctx, copyDNS, &client.UpdateOptions{}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) setEndpointFromTargets(dnsName string, dnsTargets map[string][]string, dnsRecord *kuadrantv1.DNSRecord) {
	currentEndpoints := make(map[string]*kuadrantv1.Endpoint, len(dnsRecord.Spec.Endpoints))
	for _, endpoint := range dnsRecord.Spec.Endpoints {
		address, ok := endpoint.GetAddress()
		if !ok {
			continue
		}
		currentEndpoints[address] = endpoint
	}
	var (
		newEndpoints []*kuadrantv1.Endpoint
		endpoint     *kuadrantv1.Endpoint
	)
	ok := false
	for _, targets := range dnsTargets {
		for _, target := range targets {
			// If the endpoint for this target does not exist, add a new one
			if endpoint, ok = currentEndpoints[target]; !ok {
				endpoint = &kuadrantv1.Endpoint{
					SetIdentifier: target,
				}
			}
			// Update the endpoint fields
			endpoint.DNSName = dnsName
			endpoint.RecordType = "A"
			endpoint.Targets = []string{target}
			endpoint.RecordTTL = 60
			endpoint.SetProviderSpecific(aws.ProviderSpecificWeight, awsEndpointWeight(len(targets)))
			newEndpoints = append(newEndpoints, endpoint)
		}
	}

	sort.Slice(newEndpoints, func(i, j int) bool {
		return newEndpoints[i].Targets[0] < newEndpoints[j].Targets[0]
	})

	dnsRecord.Spec.Endpoints = newEndpoints
}

// awsEndpointWeight returns the weight Value for a single AWS record in a set of records where the traffic is split
// evenly between a number of clusters/ingresses, each splitting traffic evenly to a number of IPs (numIPs)
//
// Divides the number of IPs by a known weight allowance for a cluster/ingress, note that this means:
// * Will always return 1 after a certain number of ips is reached, 60 in the current case (maxWeight / 2)
// * Will return values that don't add up to the total maxWeight when the number of ingresses is not divisible by numIPs
//
// The aws weight value must be an integer between 0 and 255.
// https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/resource-record-sets-values-weighted.html#rrsets-values-weighted-weight
func awsEndpointWeight(numIPs int) string {
	maxWeight := 120
	if numIPs > maxWeight {
		numIPs = maxWeight
	}
	return strconv.Itoa(maxWeight / numIPs)
}
