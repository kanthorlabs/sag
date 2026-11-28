package config

import (
	"testing"
)

func withStorage(block string) string {
	return minimal + block
}

func TestAWSRefNeedsStorage(t *testing.T) {
	text := `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources:
  main:
    kind: postgres
    connection: aws-sm:sag/main/conn
    agents: [agent-a]
`
	mustReject(t, text, "aws-sm")
}

func TestCredentialsSecretOnly(t *testing.T) {
	text := `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
storage:
  aws:
    region: us-east-1
    credentials: aws-sm:sag/creds
resources:
  main:
    kind: postgres
    connection: secret:sag-main
    agents: [agent-a]
`
	mustReject(t, text, "storage.aws.credentials")
}

func TestAWSRefWithStorage(t *testing.T) {
	text := `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
storage:
  aws:
    region: us-east-1
    credentials: secret:sag-aws-creds
resources:
  main:
    kind: postgres
    connection: aws-sm:sag/main/conn
    agents: [agent-a]
`
	cfg := mustParse(t, text)
	if cfg.Storage.AWS.Region != "us-east-1" {
		t.Errorf("Storage.AWS.Region = %q, want us-east-1", cfg.Storage.AWS.Region)
	}
	if string(cfg.Storage.AWS.Credentials) != "sag-aws-creds" {
		t.Errorf("Storage.AWS.Credentials = %q, want sag-aws-creds", cfg.Storage.AWS.Credentials)
	}
	r := cfg.Resources["main"]
	if string(r.Connection) != "aws-sm:sag/main/conn" {
		t.Errorf("Connection = %q, want aws-sm:sag/main/conn", r.Connection)
	}
}

func TestSecretNamesIncludesAWSCredentials(t *testing.T) {
	text := `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
storage:
  aws:
    region: us-east-1
    credentials: secret:sag-aws-creds
resources:
  main:
    kind: postgres
    connection: aws-sm:sag/main/conn
    agents: [agent-a]
`
	cfg := mustParse(t, text)
	names := cfg.SecretNames()
	found := false
	for _, n := range names {
		if n == "sag-aws-creds" {
			found = true
		}
	}
	if !found {
		t.Errorf("SecretNames missing sag-aws-creds: %v", names)
	}
	for _, n := range names {
		if n == "aws-sm:sag/main/conn" {
			t.Errorf("SecretNames must not contain aws-sm ref %q", n)
		}
	}
}
