package kfr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	k8serror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func kubeconfigPath() string {
	if kubeconfig, ok := os.LookupEnv("KUBECONFIG"); ok {
		return kubeconfig
	}

	return filepath.Join(homedir.HomeDir(), ".kube", "config")

}

func newClientset() (*kubernetes.Clientset, error) {
	kubeconfig := kubeconfigPath()
	config, _ := clientcmd.BuildConfigFromFlags("", kubeconfig)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}
	return clientset, nil
}

func newDynamicClient() (*dynamic.DynamicClient, error) {
	kubeconfig := kubeconfigPath()
	config, _ := clientcmd.BuildConfigFromFlags("", kubeconfig)

	clientset, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}
	return clientset, nil
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	select {
	case <-ctx.Done():
		t.Stop()
		return ctx.Err()
	case <-t.C:
	}
	return nil
}

func waitForTermination(ctx context.Context, timeout time.Duration, fn func() error) (bool, error) {
	ctx, _ = context.WithTimeout(ctx, timeout)
	for {
		err := fn()
		var statusErr *k8serror.StatusError
		if err != nil {
			if errors.As(err, &statusErr) && statusErr.Status().Code == 404 {
				return true, nil
			} else {
				return false, err
			}
		}

		err = sleep(ctx, 1*time.Second)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return false, nil
			} else {
				return false, err
			}
		}
	}
}
