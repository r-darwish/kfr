package kfr

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/sourcegraph/conc/pool"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func getCrdGVRs(unstructured *unstructured.Unstructured) ([]schema.GroupVersionResource, error) {
	crd := new(apiextensionsv1.CustomResourceDefinition)
	var result []schema.GroupVersionResource

	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured.Object, crd)
	if err != nil {
		return result, err
	}

	for _, version := range crd.Spec.Versions {
		result = append(result, schema.GroupVersionResource{
			Group:    crd.Spec.Group,
			Version:  version.Name,
			Resource: crd.Spec.Names.Plural,
		})

	}

	return result, nil
}

func deleteFluxLeftovers(ctx context.Context, dynamicClient *dynamic.DynamicClient, namespaces []string) error {
	crdClient := dynamicClient.Resource(crdGVR())

	crdList, err := crdClient.List(ctx, v1.ListOptions{})
	if err != nil {
		return err
	}

	pool := pool.NewWithResults[struct{}]().WithContext(ctx)

	for _, crd := range crdList.Items {
		if !strings.HasSuffix(crd.GetName(), "toolkit.fluxcd.io") {
			continue
		}
		slog.Info("Found toolkit CRD", "name", crd.GetName())

		gvrs, err := getCrdGVRs(&crd)
		if err != nil {
			return err
		}

		for _, gvr := range gvrs {
			pool.Go(func(ctx context.Context) (struct{}, error) {
				err := purgeCRD(ctx, dynamicClient, gvr, namespaces)
				if err != nil {
					return struct{}{}, fmt.Errorf("error purging CRD %s: %w", gvr.String(), err)
				}

				return struct{}{}, nil
			})
		}
	}

	_, err = pool.Wait()
	return err
}
