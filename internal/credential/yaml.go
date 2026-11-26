package credential

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
)

func checkResourceAbsent(cfgDir, name string) error {
	cfg, err := loadSAGConfig(cfgDir)
	if err != nil {
		return err
	}
	if _, exists := cfg.Resources[name]; exists {
		return fmt.Errorf("resource %s exists; delete it first", name)
	}
	return nil
}

func loadSAGConfig(cfgDir string) (*config.Config, error) {
	path := filepath.Join(cfgDir, "sag.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &config.Config{Resources: map[string]config.Resource{}}, nil
		}
		return nil, err
	}
	cfg, err := config.Parse(data)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

type resourceYAML struct {
	Kind        string   `yaml:"kind"`
	Description string   `yaml:"description,omitempty"`
	Connection  string   `yaml:"connection"`
	TLS         string   `yaml:"tls,omitempty"`
	TLSCA       string   `yaml:"tls_ca,omitempty"`
	Agents      []string `yaml:"agents"`
}

func addResourceToYAML(cfgDir, name, kind, description string, tls core.TLSMode, agents []string, connRef, caRef string, hasCA bool) error {
	path := filepath.Join(cfgDir, "sag.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read sag.yaml: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse sag.yaml: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return fmt.Errorf("sag.yaml: unexpected structure")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("sag.yaml: root is not a mapping")
	}

	res := resourceYAML{
		Kind:        kind,
		Description: description,
		Connection:  "secret:" + connRef,
		TLS:         string(tls),
		Agents:      agents,
	}
	if hasCA {
		res.TLSCA = "secret:" + caRef
	}

	var resNode yaml.Node
	if err := resNode.Encode(res); err != nil {
		return fmt.Errorf("encode resource: %w", err)
	}
	resNode.Style = 0

	if err := setMappingKey(root, "resources", name, &resNode); err != nil {
		return fmt.Errorf("update resources: %w", err)
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshal sag.yaml: %w", err)
	}

	if _, err := config.Parse(out); err != nil {
		return fmt.Errorf("validate sag.yaml: %w", err)
	}

	tmp := path + ".new"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("write sag.yaml.new: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename sag.yaml.new: %w", err)
	}
	return nil
}

func restoreYAML(cfgDir string, snapshot []byte) error {
	if snapshot == nil {
		return nil
	}
	path := filepath.Join(cfgDir, "sag.yaml")
	tmp := path + ".new"
	if err := os.WriteFile(tmp, snapshot, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func snapshotYAML(cfgDir string) ([]byte, error) {
	path := filepath.Join(cfgDir, "sag.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

func setMappingKey(root *yaml.Node, mapKey, entryKey string, value *yaml.Node) error {
	for i := 0; i+1 < len(root.Content); i += 2 {
		k := root.Content[i]
		v := root.Content[i+1]
		if k.Value != mapKey {
			continue
		}
		if v.Kind != yaml.MappingNode {
			v.Kind = yaml.MappingNode
			v.Tag = "!!map"
			v.Value = ""
			v.Style = 0
			v.Content = nil
		}
		v.Style = 0
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entryKey}
		v.Content = append(v.Content, keyNode, value)
		return nil
	}
	return fmt.Errorf("key %q not found in sag.yaml", mapKey)
}
