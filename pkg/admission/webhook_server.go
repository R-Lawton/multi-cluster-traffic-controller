package admission

import (
	"context"
	"errors"
	"fmt"

	"github.com/Kuadrant/multi-cluster-traffic-controller/pkg/admission/dnsrecord"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"net/http"
)

func CreateServer(ctx context.Context, ServerPort *int) error {

	mux := http.NewServeMux()

	handler, err := dnsrecord.CreateHandler()
	if err != nil {
		log.Error("Error creating handler", err)
		return err
	}
	webhook := &webhook.Admission{
		Handler: handler,
	}
	log.Info("Port", *ServerPort)
	mux.Handle("/dnsRecord", webhook)
	httpErr := make(chan error)
	go func() {
		httpErr <- http.ListenAndServe(fmt.Sprintf(":%d", ServerPort), mux)
	}()

	select {
	case err := <-httpErr:
		return err
	case <-ctx.Done():
		ctxErr := ctx.Err()
		if errors.Is(ctxErr, context.Canceled) {
			return nil
		}

		return ctxErr
	}
}
