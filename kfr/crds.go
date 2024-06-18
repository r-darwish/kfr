package kfr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sourcegraph/conc/pool"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func deleteCustomResource(ctx context.Context, dynamicClient *dynamic.DynamicClient, name string, crd schema.GroupVersionResource, namespace string) error {
	slog.Info("Deleting custom resource", "name", name, "namespace", namespace, "crd", crd.String())
	err := dynamicClient.Resource(crd).Namespace(namespace).Delete(ctx, name, v1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("error deleting custom resource %s in namespace %s: %w", name, namespace, err)
	}

	terminated, err := waitForTermination(ctx, time.Second*30, func() error {
		_, err := dynamicClient.Resource(crd).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
		return err
	})

	if err != nil {
		return fmt.Errorf("error waiting for custom resource %s to terminate: %w", name, err)
	}

	if terminated {
		return nil
	}

	slog.WarnContext(ctx, "Custom resource did not terminate in time. Deleting its finializers", "name", name, "namespace", namespace)

	obj, err := dynamicClient.Resource(crd).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error getting custom resource %s in namespace %s: %w", name, namespace, err)
	}
	obj.SetFinalizers([]string{})
	_, err = dynamicClient.Resource(crd).Namespace(namespace).Update(ctx, obj, v1.UpdateOptions{})
	if err != nil {
		var statusErr *k8serror.StatusError
		if errors.As(err, &statusErr) && (statusErr.Status().Code == 404 || statusErr.Status().Code == 409) {
			return nil
		}
		return fmt.Errorf("error removing finalizers from custom resource %s in namespace %s: %w", name, namespace, err)
	}

	terminated, err = waitForTermination(ctx, time.Second*30, func() error {
		_, err := dynamicClient.Resource(crd).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
		return err
	})

	if err != nil {
		return fmt.Errorf("error waiting for custom resource %s to terminate: %w", name, err)
	}

	if !terminated {
		return fmt.Errorf("custom resource %s did not terminate in time", name)
	}

	return nil
}

func purgeCRD(ctx context.Context, dynamicClient *dynamic.DynamicClient, crd schema.GroupVersionResource, namespaces []string) error {
	pool := pool.NewWithResults[struct{}]().WithContext(ctx)
	for _, namespace := range namespaces {
		list, err := dynamicClient.Resource(crd).Namespace(namespace).List(ctx, v1.ListOptions{})
		if err != nil {
			var statusErr *k8serror.StatusError
			if errors.As(err, &statusErr) && statusErr.Status().Code == 404 {
				continue
			}
			return err
		}

		for _, item := range list.Items {
			pool.Go(func(ctx context.Context) (struct{}, error) {
				return struct{}{}, deleteCustomResource(ctx, dynamicClient, item.GetName(), crd, namespace)
			})
		}
	}

	crdName := fmt.Sprintf("%s.%s", crd.Resource, crd.Group)
	_, err := pool.Wait()
	if err != nil {
		return fmt.Errorf("error purging CRD %s: %w", crdName, err)
	}

	err = dynamicClient.Resource(crdGVR()).Delete(ctx, crdName, v1.DeleteOptions{})
	if err != nil {
		var statusErr *k8serror.StatusError
		if errors.As(err, &statusErr) && statusErr.Status().Code == 404 {
			return nil
		}
		return fmt.Errorf("error deleting CRD %s: %w", crdName, err)
	}

	return err
}

func crdGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
}
