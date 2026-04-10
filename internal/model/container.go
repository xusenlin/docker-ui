package model

import (
	"time"
)

type ContainerSummary struct {
	ID      string
	Name    string
	Image   string
	Status  string
	State   string
	Ports   []PortMapping
	Created time.Time
}

type ContainerDetail struct {
	ID            string
	Name          string
	Image         string
	Status        string
	State         string
	Created       time.Time
	Ports         []PortMapping
	EnvVars       []EnvVar
	Mounts        []Mount
	Networks      []string
	Cmd           string
	WorkingDir    string
	Memory        int64
	CPUQuota      int64
	RestartPolicy string
}

type PortMapping struct {
	ContainerPort string
	HostPort      string
	Protocol      string
}

type EnvVar struct {
	Key   string
	Value string
}

type Mount struct {
	Type        string
	Source      string
	Destination string
	Mode        string
}

type ContainerRecreateConfig struct {
	Name          string
	Image         string
	State         string
	EnvVars       []string
	Cmd           []string
	WorkingDir    string
	NetworkMode   string
	PortBindings  map[string][]PortBinding
	Mounts        []MountConfig
	RestartPolicy string
	Memory        int64
	CPUQuota      int64
	Privileged    bool
	User          string
	Hostname      string
}

type PortBinding struct {
	HostIP   string
	HostPort string
}

type MountConfig struct {
	Source      string
	Destination string
	Mode        string
	ReadOnly    bool
}
