package mcpserver

type ResourceInfo struct {
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

type ListResourcesOutput struct {
	Resources []ResourceInfo `json:"resources"`
}
