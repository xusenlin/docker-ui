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
	Entrypoint    []string
	WorkingDir    string
	NetworkMode   string
	PortBindings  map[string][]PortBinding
	Mounts        []MountConfig
	RestartPolicy string
	Memory        int64
	CPUQuota      int64
	CPUPeriod     int64
	CPUShares     int64
	Privileged    bool
	User          string
	Hostname      string
	Domainname    string
	Labels        map[string]string
	CapAdd        []string
	CapDrop       []string
	DNS           []string
	DNSSearch     []string
	ExtraHosts    []string
	LogConfig     LogConfig
	PIDMode       string
	IPCMode       string
	SecurityOpt   []string
	Tmpfs         map[string]string
	Sysctls       map[string]string
	TTY           bool
	OpenStdin     bool
	Healthcheck   *HealthcheckConfig
	NetworkConfig *NetworkConfig
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

type LogConfig struct {
	Type   string
	Config map[string]string
}

type HealthcheckConfig struct {
	Test        []string
	Interval    int64
	Timeout     int64
	StartPeriod int64
	Retries     int64
}

type NetworkConfig struct {
	NetworkID string
	Aliases   []string
	IPv4      string
	IPv6      string
}
