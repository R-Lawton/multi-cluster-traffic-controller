package dnsrecord

import (
	"context"
	// corev1 "k8s.io/api/core/v1"
	// "net/http"

	kuadrantiov1 "github.com/Kuadrant/multi-cluster-traffic-controller/pkg/apis/v1"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type Handler struct {
	Decoder *admission.Decoder
	client.Client
}

func CreateHandler() (admission.Handler, error) {
	log.Info("Creating handler ")
	scheme := runtime.NewScheme()
	if err := kuadrantiov1.AddToScheme(scheme); err != nil {
		log.Error("Unable add to scheme", err)
		return nil, err
	}

	log.Info("Creating decoder")
	decoder, err := admission.NewDecoder(scheme)
	if err != nil {
		log.Error("Unable to create decoder", err)
		return nil, err
	}
	return &Handler{
		Decoder: decoder,
	}, nil

}
func (h *Handler) Handle(ctx context.Context, req admission.Request) admission.Response {
	// pod := &corev1.Pod{}
	// err := h.Decoder.Decode(req, pod)
	// if err != nil {
	// 	return admission.Errored(http.StatusBadRequest, err)
	// }
	// if pod.Name != "test" {
	// 	admission.Denied("Name doesnt match")
	// }
	return admission.Allowed("names match")
}

