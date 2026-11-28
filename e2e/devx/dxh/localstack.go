//go:build devxe2e

package dxh

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
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

	t.Setenv("SAG_TEST_AWS_ENDPOINT", localStackInner)
	t.Setenv("AWS_ENDPOINT_URL_SECRETS_MANAGER", localStackOuter)
	t.Setenv("AWS_ACCESS_KEY_ID", localStackAccessKey)
	t.Setenv("AWS_SECRET_ACCESS_KEY", localStackSecretKey)
	t.Setenv("AWS_REGION", localStackRegion)
	t.Setenv("AWS_CONFIG_FILE", os.DevNull)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", os.DevNull)
}

func localStackRunning(ctx context.Context, r Runner) bool {
	out, err := r.Output(ctx, "podman", "container", "inspect", localStackName, "--format", "{{.State.Running}}")
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

const LocalStackName = localStackName

func startLocalStack(ctx context.Context, r Runner) error {
	args := []string{"run", "--detach", "--rm",
		"--label", "sag.devx-e2e=1",
		"--name", localStackName,
		"--publish", "127.0.0.1:4566:4566",
	}
	if runtime.GOOS == "linux" {
		addr, err := PublishAddr(ctx, r)
		if err != nil {
			return err
		}
		args = append(args, "--publish", addr+":4566:4566")
	}
	args = append(args,
		"--env", "SERVICES=secretsmanager",
		"--env", "DEFAULT_REGION="+localStackRegion,
		"--env", "AWS_DEFAULT_REGION="+localStackRegion,
		LocalStackImage,
	)
	_, err := r.Output(ctx, "podman", args...)
	return err
}

func waitLocalStackReady(ctx context.Context) error {
	deadline := time.Now().Add(localStackReadyTimeout)
	for {
		conn, err := net.DialTimeout("tcp", localStackHostAddr, time.Second)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("localstack tcp not ready after %s", localStackReadyTimeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
	client := LocalStackClient()
	for {
		_, err := client.ListSecrets(ctx, &secretsmanager.ListSecretsInput{})
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("localstack secretsmanager not ready after %s: %w", localStackReadyTimeout, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func LocalStackClient() *secretsmanager.Client {
	return secretsmanager.New(secretsmanager.Options{
		Region:       localStackRegion,
		Credentials:  credentials.NewStaticCredentialsProvider(localStackAccessKey, localStackSecretKey, ""),
		BaseEndpoint: aws.String(localStackOuter),
	})
}

func LocalStackCreateSecret(ctx context.Context, name, value string) error {
	_, err := LocalStackClient().CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		SecretString: aws.String(value),
	})
	return err
}

func LocalStackDeleteSecret(ctx context.Context, name string) error {
	_, err := LocalStackClient().DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(name),
		ForceDeleteWithoutRecovery: aws.Bool(true),
	})
	return err
}

func LocalStackTags(ctx context.Context, name string) (map[string]string, error) {
	out, err := LocalStackClient().DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)})
	if err != nil {
		return nil, err
	}
	tags := make(map[string]string, len(out.Tags))
	for _, tag := range out.Tags {
		k, v := aws.ToString(tag.Key), aws.ToString(tag.Value)
		if _, dup := tags[k]; dup {
			return nil, fmt.Errorf("secret %s: duplicate tag %q", name, k)
		}
		tags[k] = v
	}
	return tags, nil
}

func LocalStackSecretExists(ctx context.Context, name string) (bool, error) {
	_, err := LocalStackClient().DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)})
	if err == nil {
		return true, nil
	}
	var nf *smtypes.ResourceNotFoundException
	if errors.As(err, &nf) {
		return false, nil
	}
	return false, err
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

const (
	awsTestPGImage = "docker.io/library/postgres:17.12-bookworm"
	awsTestPGPort  = "5432/tcp"
)

func StartAWSTestPostgres(ctx context.Context, r Runner, name string) (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("StartAWSTestPostgres: rand: %w", err)
	}
	sfx := hex.EncodeToString(b)
	user := "u" + sfx[:8]
	pwd := "p" + sfx[8:16]
	database := "d" + sfx[16:]
	addr, err := PublishAddr(ctx, r)
	if err != nil {
		return "", fmt.Errorf("StartAWSTestPostgres: %w", err)
	}
	_, err = r.Output(ctx,
		"podman", "run", "--detach", "--rm",
		"--label", "sag.devx-e2e=1",
		"--name", name,
		"--publish", addr+"::5432",
		"--env", "POSTGRES_USER="+user,
		"--env", "POSTGRES_PASSWORD="+pwd,
		"--env", "POSTGRES_DB="+database,
		awsTestPGImage,
	)
	if err != nil {
		return "", fmt.Errorf("StartAWSTestPostgres: start: %w", err)
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		if _, err := r.Output(ctx, "podman", "exec", name, "pg_isready", "-U", user, "-h", "127.0.0.1"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("StartAWSTestPostgres: %s not ready", name)
		}
		time.Sleep(500 * time.Millisecond)
	}
	out, err := r.Output(ctx, "podman", "port", name, awsTestPGPort)
	if err != nil {
		return "", fmt.Errorf("StartAWSTestPostgres: port: %w", err)
	}
	line := strings.TrimSpace(string(out))
	i := strings.LastIndex(line, ":")
	if i < 0 {
		return "", fmt.Errorf("StartAWSTestPostgres: unexpected port output: %q", line)
	}
	port := line[i+1:]
	dsn := "postgres://" + user + ":" + pwd + "@host.containers.internal:" + port + "/" + database
	return dsn, nil
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
