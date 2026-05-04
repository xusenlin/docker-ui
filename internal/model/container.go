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

type NetworkConfig struct {
	NetworkID string
	Aliases   []string
	IPv4      string
	IPv6      string
}
