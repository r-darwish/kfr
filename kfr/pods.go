package kfr

import (
	"context"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func deleteAllPods(ctx context.Context, clientset *kubernetes.Clientset, namespaces []string) error {
	for _, namespace := range namespaces {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, v1.ListOptions{})

		if err != nil {
			return fmt.Errorf("error getting pods in namespace %s: %w", namespace, err)
		}

		for _, pod := range pods.Items {
			slog.Info("Deleting pod", "name", pod.Name, "namespace", namespace)
			err := clientset.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: new(int64)})

			if err != nil {
				return fmt.Errorf("error deleting pod %s in namespace %s: %w", pod.Name, namespace, err)
			}
		}
	}

	return nil
}
