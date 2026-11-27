package hoststate

type Paths struct {
	Cfg         string
	SAGYaml     string
	CAPem       string
	AnswersJSON string
	Lock        string
}

type Answers struct {
	Autostart    string            `json:"autostart,omitempty"`
	AgentSetup   map[string]string `json:"agent_setup,omitempty"`
	DenyRules    map[string]string `json:"deny_rules,omitempty"`
	ShellProfile map[string]string `json:"shell_profile,omitempty"`
	Guard        map[string]string `json:"guard,omitempty"`
	PodmanPath   string            `json:"podman_path,omitempty"`
	TailscaleIP  string            `json:"tailscale_ip,omitempty"`
}
