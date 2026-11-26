package podman

import "time"

type SecretInfo struct {
	ID        string
	Spec      SecretSpec
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SecretSpec struct {
	Name   string
	Labels map[string]string
}

type ContainerInfo struct {
	ID      string
	Name    string
	Secrets []ContainerSecret
}

type ContainerSecret struct {
	Name string
	UID  int
	GID  int
	Mode uint32
}
