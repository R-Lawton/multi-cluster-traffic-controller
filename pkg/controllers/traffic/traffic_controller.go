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
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kuadrantv1 "github.com/Kuadrant/multi-cluster-traffic-controller/pkg/apis/v1"
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

	return ctrl.Result{}, nil
}
