package kfr

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/sourcegraph/conc/pool"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func getNamespaces(ctx context.Context, clientset *kubernetes.Clientset) ([]string, error) {
	namespaceList, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve namespaces: %w", err)
	}

	namespaces := make([]string, 0, len(namespaceList.Items))
	for _, namespace := range namespaceList.Items {
		if namespace.Name == "kube-system" || namespace.Name == "kube-public" || namespace.Name == "kube-node-lease" {
			continue
		}

		namespaces = append(namespaces, namespace.Name)
	}

	return namespaces, nil

}

func getNonDefaultNamespaces(ctx context.Context, clientset *kubernetes.Clientset) ([]string, error) {
	namespaceList, err := getNamespaces(ctx, clientset)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve namespaces: %w", err)
	}

	namespaces := make([]string, 0, len(namespaceList))
	for _, namespace := range namespaceList {
		if namespace == "default" {
			continue
		}

		namespaces = append(namespaces, namespace)
	}

	return namespaces, nil
}

func deleteNamespace(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	slog.InfoContext(ctx, "deleting namespace", "namespace", namespace)
	err := clientset.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete namespace %s: %w", namespace, err)
	}

	terminated, err := waitForTermination(ctx, 10*time.Second, func() error {
		_, err := clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		return err
	})

	if err != nil {
		return fmt.Errorf("error waiting for namespace to be deleted: %w", err)
	}

	if !terminated {
		return fmt.Errorf("namespace %s was not deleted", namespace)
	}

	return nil
}

func deleteNamespaces(ctx context.Context, clientset *kubernetes.Clientset) error {
	namespaces, err := getNonDefaultNamespaces(ctx, clientset)
	if err != nil {
		return fmt.Errorf("error getting namespaces: %w", err)
	}

	pool := pool.NewWithResults[struct{}]().WithContext(ctx)
	for _, namespace := range namespaces {
		pool.Go(func(ctx context.Context) (struct{}, error) {
			err := deleteNamespace(ctx, clientset, namespace)
			return struct{}{}, err
		})
	}

	_, err = pool.Wait()
	if err != nil {
		return fmt.Errorf("error deleting namespaces: %w", err)
	}

	return nil
}
