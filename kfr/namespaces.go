package kfr

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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

func deleteNamespaces(ctx context.Context, clientset *kubernetes.Clientset) error {
	ctx, _ = context.WithTimeout(ctx, 10*time.Second)

	for {
		namespaces, err := getNonDefaultNamespaces(ctx, clientset)
		if err != nil {
			return fmt.Errorf("error getting namespaces: %w", err)
		}

		if len(namespaces) == 0 {
			break
		}

		for _, namespace := range namespaces {
			slog.InfoContext(ctx, "deleting namespace", "namespace", namespace)
			err := clientset.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
			if err != nil {
				return fmt.Errorf("failed to delete namespace %s: %w", namespace, err)
			}
		}

		err = sleep(ctx, 1*time.Second)
		if err != nil {
			return err
		}
	}

	return nil
}
