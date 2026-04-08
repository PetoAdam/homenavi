package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

	"github.com/PetoAdam/homenavi/integration-proxy/internal/config"
	"github.com/PetoAdam/homenavi/shared/envx"
)

type environmentRuntime string

const (
	runtimeCompose environmentRuntime = "compose"
	runtimeHelm    environmentRuntime = "helm"
	runtimeGitOps  environmentRuntime = "gitops"
)

func (s *Server) runtimeMode() (environmentRuntime, error) {
	mode := strings.ToLower(envx.String("INTEGRATIONS_RUNTIME_MODE", ""))
	switch mode {
	case "", "auto":
		if envx.String("KUBERNETES_SERVICE_HOST", "") != "" {
			return runtimeHelm, nil
		}
		if s.composeEnabled() {
			return runtimeCompose, nil
		}
		return "", fmt.Errorf("unable to auto-detect runtime mode; set INTEGRATIONS_RUNTIME_MODE to compose, helm, or gitops")
	case string(runtimeCompose):
		return runtimeCompose, nil
	case string(runtimeHelm):
		return runtimeHelm, nil
	case string(runtimeGitOps):
		return runtimeGitOps, nil
	default:
		return "", fmt.Errorf("invalid INTEGRATIONS_RUNTIME_MODE %q; expected compose, helm, gitops, or auto", mode)
	}
}

func (s *Server) ensureMutableRuntime() (environmentRuntime, error) {
	mode, err := s.runtimeMode()
	if err != nil {
		return "", err
	}
	if mode == runtimeGitOps {
		return "", fmt.Errorf("gitops mode is read-only; manage integrations from GitOps repository")
	}
	return mode, nil
}

func (s *Server) managementModeLabel() string {
	mode, err := s.runtimeMode()
	if err != nil {
		return "unknown"
	}
	return string(mode)
}

func (s *Server) defaultHelmNamespace() string {
	ns := envx.String("INTEGRATIONS_HELM_NAMESPACE", "")
	if ns == "" {
		ns = "homenavi-integrations"
	}
	return ns
}

func (s *Server) defaultHelmReleaseName(id string) string {
	prefix := envx.String("INTEGRATIONS_HELM_RELEASE_PREFIX", "")
	if prefix == "" {
		prefix = "homenavi-int-"
	}
	normalized := normalizeKubeName(id)
	if normalized == "" {
		normalized = "integration"
	}
	return prefix + normalized
}

func normalizeKubeName(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(normalized, "-")
	return strings.Trim(normalized, "-")
}

func chartNameFromRef(chartRef string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(chartRef), "/")
	if trimmed == "" {
		return ""
	}
	return normalizeKubeName(path.Base(trimmed))
}

func helmServiceName(releaseName, chartRef string) string {
	releaseName = normalizeKubeName(releaseName)
	if releaseName == "" {
		return ""
	}
	chartName := chartNameFromRef(chartRef)
	if chartName == "" {
		return releaseName
	}
	return releaseName + "-" + chartName
}

func (s *Server) runHelm(ctx context.Context, args ...string) error {
	bin := envx.String("INTEGRATIONS_HELM_BIN", "")
	if bin == "" {
		bin = "helm"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Server) runKubectl(ctx context.Context, args ...string) error {
	bin := envx.String("INTEGRATIONS_KUBECTL_BIN", "")
	if bin == "" {
		bin = "kubectl"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

type helmDeploymentSpec struct {
	ReleaseName  string
	Namespace    string
	ChartRef     string
	ChartVersion string
	ValuesFile   string
}

func (s *Server) helmSpecFor(id string, ic config.IntegrationConfig, target *marketplaceIntegration) (helmDeploymentSpec, error) {
	release := strings.TrimSpace(ic.HelmReleaseName)
	if release == "" {
		release = s.defaultHelmReleaseName(id)
	}
	namespace := strings.TrimSpace(ic.HelmNamespace)
	if namespace == "" {
		namespace = strings.TrimSpace(ic.DevHelmNamespace)
	}
	if namespace == "" {
		namespace = s.defaultHelmNamespace()
	}
	chartRef := strings.TrimSpace(ic.HelmChartRef)
	chartVersion := strings.TrimSpace(ic.HelmChartVersion)
	if target != nil {
		targetChart, targetVersion := target.preferredHelmArtifact()
		if chartRef == "" {
			chartRef = strings.TrimSpace(targetChart)
		}
		if chartVersion == "" {
			chartVersion = strings.TrimSpace(targetVersion)
		}
	}
	if chartRef == "" {
		chartRef = strings.TrimSpace(ic.DevHelmChartRef)
	}
	if chartVersion == "" {
		chartVersion = strings.TrimSpace(ic.DevHelmVersion)
	}
	if chartRef == "" {
		return helmDeploymentSpec{}, fmt.Errorf("helm chart artifact unavailable for integration %q", id)
	}
	return helmDeploymentSpec{
		ReleaseName:  release,
		Namespace:    namespace,
		ChartRef:     chartRef,
		ChartVersion: chartVersion,
		ValuesFile:   strings.TrimSpace(ic.HelmValuesFile),
	}, nil
}

func (s *Server) helmInstallOrUpgrade(ctx context.Context, spec helmDeploymentSpec, id string) error {
	if strings.TrimSpace(spec.ChartRef) == "" {
		return fmt.Errorf("missing helm chart ref")
	}
	s.setInstallStatus(id, "pulling", 35, "Resolving Helm chart")
	args := []string{"upgrade", "--install", spec.ReleaseName, spec.ChartRef, "--namespace", spec.Namespace, "--create-namespace", "--wait", "--atomic"}
	if strings.TrimSpace(spec.ChartVersion) != "" {
		args = append(args, "--version", spec.ChartVersion)
	}
	if strings.TrimSpace(spec.ValuesFile) != "" {
		args = append(args, "-f", spec.ValuesFile)
	}
	args = append(args, s.helmCommonEnvArgs()...)
	args = append(args, "--set", "persistence.enabled=true")
	s.setInstallStatus(id, "starting", 55, "Deploying with Helm")
	return s.runHelm(ctx, args...)
}

func (s *Server) helmCommonEnvArgs() []string {
	args := s.helmInlineJWTArgs()
	if brokerURL := strings.TrimSpace(s.defaultHelmMQTTBrokerURL()); brokerURL != "" {
		args = append(args, "--set-string", fmt.Sprintf("env.MQTT_BROKER_URL=%s", brokerURL))
	}
	return args
}

func (s *Server) helmInlineJWTArgs() []string {
	publicKeyPath := envx.String("JWT_PUBLIC_KEY_PATH", "")
	if publicKeyPath == "" {
		return nil
	}
	info, err := os.Stat(publicKeyPath)
	if err != nil || info.IsDir() {
		return nil
	}
	return []string{"--set-file", fmt.Sprintf("env.JWT_PUBLIC_KEY=%s", publicKeyPath)}
}

func (s *Server) defaultHelmMQTTBrokerURL() string {
	if value := envx.String("INTEGRATIONS_HELM_MQTT_BROKER_URL", ""); value != "" {
		return value
	}
	coreNamespace := envx.String("INTEGRATIONS_HELM_CORE_NAMESPACE", "")
	if coreNamespace == "" {
		coreNamespace = "homenavi"
	}
	return fmt.Sprintf("mqtt://mosquitto.%s.svc.cluster.local:1883", coreNamespace)
}

func (s *Server) helmUninstall(ctx context.Context, releaseName, namespace string) error {
	releaseName = strings.TrimSpace(releaseName)
	namespace = strings.TrimSpace(namespace)
	if releaseName == "" {
		return fmt.Errorf("missing helm release name")
	}
	if namespace == "" {
		namespace = s.defaultHelmNamespace()
	}
	err := s.runHelm(ctx, "uninstall", releaseName, "--namespace", namespace)
	if err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "release: not found") || strings.Contains(msg, "not found") {
			return nil
		}
	}
	return err
}

func (s *Server) helmRestart(ctx context.Context, releaseName, namespace string) error {
	releaseName = strings.TrimSpace(releaseName)
	namespace = strings.TrimSpace(namespace)
	if releaseName == "" {
		return fmt.Errorf("missing helm release name")
	}
	if namespace == "" {
		namespace = s.defaultHelmNamespace()
	}
	selector := fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName)
	return s.runKubectl(ctx, "-n", namespace, "rollout", "restart", "deployment", "-l", selector)
}
