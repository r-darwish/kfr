package kfr

import (
	"context"
	"fmt"
	"os"

	"github.com/sourcegraph/conc/pool"
	"golang.org/x/term"
)

func Purge() error {
	ctx := context.Background()

	currentContext, err := getCurrentContext()
	if err != nil {
		return fmt.Errorf("error getting cluster name: %w", err)
	}

	if term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Printf("Purging cluster %s with user %s. Press enter to continue", currentContext.Cluster, currentContext.AuthInfo)
		fmt.Scanln()
	}

	clientset, err := newClientset()
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	namespaces, err := getNamespaces(ctx, clientset)
	if err != nil {
		return fmt.Errorf("error getting namespaces: %w", err)
	}

	pool := pool.NewWithResults[struct{}]().WithContext(ctx)
	for _, namespace := range namespaces {
		namespace := namespace
		pool.Go(func(ctx context.Context) (struct{}, error) {
			err := uninstallHelmCharts(ctx, namespace, clientset)
			return struct{}{}, err
		})
	}

	_, err = pool.Wait()
	if err != nil {
		return fmt.Errorf("error purging helm: %w", err)
	}

	dynamic, err := newDynamicClient()
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	err = deleteFluxLeftovers(ctx, dynamic, namespaces)
	if err != nil {
		return fmt.Errorf("error deleting flux leftovers: %w", err)
	}

	err = deleteAllPods(ctx, clientset, namespaces)
	if err != nil {
		return fmt.Errorf("error deleting pods: %w", err)
	}

	err = deleteNamespaces(ctx, clientset)
	if err != nil {
		return fmt.Errorf("error deleting namespaces: %w", err)
	}

	return nil
}
