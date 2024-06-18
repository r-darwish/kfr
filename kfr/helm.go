package kfr

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	helmclient "github.com/mittwald/go-helm-client"
	"github.com/sourcegraph/conc/pool"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func releasesToCharts(releases []*release.Release) []string {
	set := make(map[string]struct{})

	for _, release := range releases {
		set[release.Name] = struct{}{}
	}

	charts := make([]string, 0, len(set))
	for chart := range set {
		charts = append(charts, chart)
	}

	return charts
}

func getDeployedCharts(client helmclient.Client) ([]string, error) {
	releases, err := client.ListReleasesByStateMask(action.ListAll)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployed releases: %w", err)
	}

	return releasesToCharts(releases), nil
}

func uninstallChart(ctx context.Context, client helmclient.Client, chart string, clientset *kubernetes.Clientset) error {
	slog.InfoContext(ctx, "uninstalling helm release", "chart", chart)

	err := client.UninstallReleaseByName(chart)
	if err == nil {
		return nil
	}
	slog.WarnContext(ctx, "failed to uninstall release. Will delete the secrets", "chart", chart)

	secretList, err := clientset.CoreV1().Secrets(client.GetSettings().Namespace()).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to secrets: %w", err)
	}

	for _, secret := range secretList.Items {
		if strings.HasPrefix(secret.Name, fmt.Sprintf("sh.helm.release.v1.%s.v", chart)) {
			slog.Info("deleting secret", "secret", secret.Name, "chart", chart, "namespace", client.GetSettings().Namespace())
			err := clientset.CoreV1().Secrets(client.GetSettings().Namespace()).Delete(ctx, secret.Name, metav1.DeleteOptions{})
			if err != nil {
				return fmt.Errorf("failed to delete secret %s: %w", secret.Name, err)
			}
		}
	}

	return nil
}

func uninstallHelmCharts(ctx context.Context, namespace string, clientset *kubernetes.Clientset) error {
	slog.InfoContext(ctx, "purging helm releases", "namespace", namespace)

	helmClient, err := helmclient.New(&helmclient.Options{Namespace: namespace})
	if err != nil {
		return fmt.Errorf("failed to create helm client: %w", err)
	}

	charts, err := getDeployedCharts(helmClient)
	if err != nil {
		return fmt.Errorf("failed to get charts: %w", err)
	}

	pool := pool.NewWithResults[struct{}]().WithContext(ctx)

	for _, chart := range charts {
		chart := chart
		pool.Go(func(ctx context.Context) (struct{}, error) {
			err := uninstallChart(ctx, helmClient, chart, clientset)
			if err != nil {
				return struct{}{}, fmt.Errorf("failed to uninstall chart %s: %w", chart, err)
			}
			return struct{}{}, nil
		})
	}

	_, err = pool.Wait()
	if err != nil {
		return fmt.Errorf("failed to uninstall releases: %w", err)
	}

	return nil
}
