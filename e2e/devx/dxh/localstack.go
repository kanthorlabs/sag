//go:build devxe2e

package dxh

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	LocalStackImage        = "docker.io/localstack/localstack@sha256:b279c01f4cfb8f985a482e4014cabc1e2697b9d7a6c8c8db2e40f4d9f93687c7"
	localStackName         = "sag-localstack"
	localStackHostAddr     = "127.0.0.1:4566"
	localStackInner        = "http://host.containers.internal:4566"
	localStackOuter        = "http://localhost:4566"
	localStackRegion       = "us-east-1"
	localStackAccessKey    = "test"
	localStackSecretKey    = "test"
	localStackReadyTimeout = 90 * time.Second
	awsCredSecretName      = "sag-aws-credentials"
)

var ensureLocalStackBootstrapFn = EnsureBootstrapped

func EnsureLocalStack(t *testing.T) {
	t.Helper()
	ensureLocalStackBootstrapFn(t)

	r := newRunnerFn()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if !localStackRunning(ctx, r) {
		if err := startLocalStack(ctx, r); err != nil {
			t.Fatalf("EnsureLocalStack: start: %v", err)
		}
	}

	if err := waitLocalStackReady(ctx); err != nil {
		t.Fatalf("EnsureLocalStack: wait ready: %v", err)
	}

	if err := ensureAWSCredSecret(ctx); err != nil {
		t.Fatalf("EnsureLocalStack: aws cred secret: %v", err)
	}

	home := os.Getenv("HOME")
	cfgDir := CfgDir(home)
	if err := ensureStorageAWS(cfgDir); err != nil {
		t.Fatalf("EnsureLocalStack: sag.yaml: %v", err)
	}

	t.Setenv("AWS_ENDPOINT_URL_SECRETS_MANAGER", localStackOuter)
	t.Setenv("AWS_ACCESS_KEY_ID", localStackAccessKey)
	t.Setenv("AWS_SECRET_ACCESS_KEY", localStackSecretKey)
	t.Setenv("AWS_REGION", localStackRegion)
}

func localStackRunning(ctx context.Context, r Runner) bool {
	out, err := r.Output(ctx, "podman", "container", "inspect", localStackName, "--format", "{{.State.Running}}")
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func startLocalStack(ctx context.Context, r Runner) error {
	_, err := r.Output(ctx,
		"podman", "run", "--detach",
		"--label", "sag.devx-e2e=1",
		"--name", localStackName,
		"--publish", "127.0.0.1:4566:4566",
		"--env", "SERVICES=secretsmanager",
		"--env", "DEFAULT_REGION="+localStackRegion,
		"--env", "AWS_DEFAULT_REGION="+localStackRegion,
		LocalStackImage,
	)
	return err
}

func waitLocalStackReady(ctx context.Context) error {
	deadline := time.Now().Add(localStackReadyTimeout)
	for {
		conn, err := net.DialTimeout("tcp", localStackHostAddr, time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("localstack not ready after %s", localStackReadyTimeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func ensureAWSCredSecret(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "podman", "secret", "ls", "--filter", "name=^"+awsCredSecretName+"$", "--format", "{{.Name}}").Output()
	if err != nil {
		return fmt.Errorf("list secret: %w", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		return nil
	}
	credContent := "[default]\naws_access_key_id = " + localStackAccessKey + "\naws_secret_access_key = " + localStackSecretKey + "\n"
	cmd := exec.CommandContext(ctx, "podman", "secret", "create",
		"--label", "sag.devx-e2e=1",
		awsCredSecretName, "-")
	cmd.Stdin = strings.NewReader(credContent)
	if o, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create aws cred secret: %w\n%s", err, o)
	}
	return nil
}

func ensureStorageAWS(cfgDir string) error {
	sagYAML := filepath.Join(cfgDir, "sag.yaml")
	data, err := os.ReadFile(sagYAML)
	if err != nil {
		return err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}

	if hasStorageKey(root) {
		return nil
	}

	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "storage"},
		buildStorageAWSNode(),
	)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}

	tmp := sagYAML + ".new"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, sagYAML)
}

func hasStorageKey(root *yaml.Node) bool {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "storage" {
			return true
		}
	}
	return false
}

func buildStorageAWSNode() *yaml.Node {
	credRef := "secret:" + awsCredSecretName
	return &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "aws"},
			{
				Kind: yaml.MappingNode,
				Tag:  "!!map",
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Tag: "!!str", Value: "region"},
					{Kind: yaml.ScalarNode, Tag: "!!str", Value: localStackRegion},
					{Kind: yaml.ScalarNode, Tag: "!!str", Value: "credentials"},
					{Kind: yaml.ScalarNode, Tag: "!!str", Value: credRef},
				},
			},
		},
	}
}
