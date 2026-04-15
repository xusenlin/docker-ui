package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"

	"docker-ui/internal/model"
)

func (c *Client) ListContainers(ctx context.Context) ([]model.ContainerSummary, error) {
	opts := container.ListOptions{All: true}
	raw, err := c.cli.ContainerList(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	summaries := make([]model.ContainerSummary, 0, len(raw))
	for _, ctn := range raw {
		name := ctn.Names[0]
		name = strings.TrimPrefix(name, "/")

		ports := make([]model.PortMapping, 0, len(ctn.Ports))
		for _, p := range ctn.Ports {
			pm := model.PortMapping{
				ContainerPort: fmt.Sprintf("%d", p.PrivatePort),
				Protocol:      "tcp",
			}
			if p.PublicPort > 0 {
				hostIP := p.IP
				if hostIP == "0.0.0.0" {
					hostIP = ""
				}
				pm.HostPort = fmt.Sprintf("%s:%d", hostIP, p.PublicPort)
			}
			ports = append(ports, pm)
		}

		summaries = append(summaries, model.ContainerSummary{
			ID:      ctn.ID[:12],
			Name:    name,
			Image:   ctn.Image,
			Status:  ctn.Status,
			State:   ctn.State,
			Ports:   ports,
			Created: time.Unix(ctn.Created, 0),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Name < summaries[j].Name
	})

	return summaries, nil
}

func (c *Client) GetContainerDetail(ctx context.Context, id string) (*model.ContainerDetail, error) {
	raw, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("inspect container %s: %w", id, err)
	}

	name := raw.Name
	name = strings.TrimPrefix(name, "/")

	ports := make([]model.PortMapping, 0)
	for containerPort, bindings := range raw.NetworkSettings.Ports {
		for _, b := range bindings {
			hostIP := b.HostIP
			if hostIP == "0.0.0.0" {
				hostIP = ""
			}
			parts := strings.SplitN(string(containerPort), "/", 2)
			proto := "tcp"
			if len(parts) > 1 {
				proto = parts[1]
			}
			ports = append(ports, model.PortMapping{
				ContainerPort: parts[0],
				HostPort:      fmt.Sprintf("%s:%s", hostIP, b.HostPort),
				Protocol:      proto,
			})
		}
	}

	envVars := make([]model.EnvVar, 0)
	for _, e := range raw.Config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars = append(envVars, model.EnvVar{Key: parts[0], Value: parts[1]})
		}
	}

	mounts := make([]model.Mount, 0, len(raw.Mounts))
	for _, m := range raw.Mounts {
		mounts = append(mounts, model.Mount{
			Type:        string(m.Type),
			Source:      m.Source,
			Destination: m.Destination,
			Mode:        m.Mode,
		})
	}

	networks := make([]string, 0, len(raw.NetworkSettings.Networks))
	for net := range raw.NetworkSettings.Networks {
		networks = append(networks, net)
	}

	cmd := strings.Join(raw.Config.Cmd, " ")
	if cmd == "" {
		cmd = "-"
	}

	workingDir := raw.Config.WorkingDir
	if workingDir == "" {
		workingDir = "-"
	}

	detail := &model.ContainerDetail{
		ID:            raw.ID[:12],
		Name:          name,
		Image:         raw.Config.Image,
		Status:        raw.State.Status,
		State:         raw.State.Status,
		Created:       parseTime(raw.Created),
		Ports:         ports,
		EnvVars:       envVars,
		Mounts:        mounts,
		Networks:      networks,
		Cmd:           cmd,
		WorkingDir:    workingDir,
		Memory:        raw.HostConfig.Memory,
		CPUQuota:      raw.HostConfig.CPUQuota,
		RestartPolicy: string(raw.HostConfig.RestartPolicy.Name),
	}

	return detail, nil
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (c *Client) StartContainer(ctx context.Context, id string) error {
	return c.cli.ContainerStart(ctx, id, container.StartOptions{})
}

func (c *Client) StopContainer(ctx context.Context, id string) error {
	return c.cli.ContainerStop(ctx, id, container.StopOptions{})
}

func (c *Client) PauseContainer(ctx context.Context, id string) error {
	return c.cli.ContainerPause(ctx, id)
}

func (c *Client) UnpauseContainer(ctx context.Context, id string) error {
	return c.cli.ContainerUnpause(ctx, id)
}

func (c *Client) GetRecreateConfig(ctx context.Context, id string) (*model.ContainerRecreateConfig, error) {
	raw, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("inspect container %s: %w", id, err)
	}

	portBindings := make(map[string][]model.PortBinding)
	for containerPort, bindings := range raw.NetworkSettings.Ports {
		for _, b := range bindings {
			portBindings[string(containerPort)] = append(portBindings[string(containerPort)], model.PortBinding{
				HostIP:   b.HostIP,
				HostPort: b.HostPort,
			})
		}
	}

	mounts := make([]model.MountConfig, 0, len(raw.Mounts))
	for _, m := range raw.Mounts {
		mounts = append(mounts, model.MountConfig{
			Source:      m.Source,
			Destination: m.Destination,
			Mode:        m.Mode,
			ReadOnly:    m.RW == false,
		})
	}

	var healthcheck *model.HealthcheckConfig
	if raw.Config.Healthcheck != nil {
		healthcheck = &model.HealthcheckConfig{
			Test:    raw.Config.Healthcheck.Test,
			Retries: int64(raw.Config.Healthcheck.Retries),
		}
		if raw.Config.Healthcheck.Interval != 0 {
			healthcheck.Interval = int64(raw.Config.Healthcheck.Interval / time.Millisecond)
		}
		if raw.Config.Healthcheck.Timeout != 0 {
			healthcheck.Timeout = int64(raw.Config.Healthcheck.Timeout / time.Millisecond)
		}
		if raw.Config.Healthcheck.StartPeriod != 0 {
			healthcheck.StartPeriod = int64(raw.Config.Healthcheck.StartPeriod / time.Millisecond)
		}
	}

	var networkConfig *model.NetworkConfig
	for netName, ep := range raw.NetworkSettings.Networks {
		aliases := make([]string, 0, len(ep.Aliases))
		for _, a := range ep.Aliases {
			if a != id[:12] && a != strings.TrimPrefix(raw.Name, "/") {
				aliases = append(aliases, a)
			}
		}
		nc := &model.NetworkConfig{
			NetworkID: ep.NetworkID,
			Aliases:   aliases,
		}
		if ep.IPAMConfig != nil {
			nc.IPv4 = ep.IPAMConfig.IPv4Address
			nc.IPv6 = ep.IPAMConfig.IPv6Address
		}
		networkConfig = nc
		_ = netName
		break
	}

	logConfig := model.LogConfig{
		Type:   raw.HostConfig.LogConfig.Type,
		Config: raw.HostConfig.LogConfig.Config,
	}

	config := &model.ContainerRecreateConfig{
		Name:          strings.TrimPrefix(raw.Name, "/"),
		Image:         raw.Config.Image,
		State:         raw.State.Status,
		EnvVars:       raw.Config.Env,
		Cmd:           raw.Config.Cmd,
		Entrypoint:    raw.Config.Entrypoint,
		WorkingDir:    raw.Config.WorkingDir,
		NetworkMode:   string(raw.HostConfig.NetworkMode),
		PortBindings:  portBindings,
		Mounts:        mounts,
		RestartPolicy: string(raw.HostConfig.RestartPolicy.Name),
		Memory:        raw.HostConfig.Memory,
		CPUQuota:      raw.HostConfig.CPUQuota,
		CPUPeriod:     raw.HostConfig.CPUPeriod,
		CPUShares:     raw.HostConfig.CPUShares,
		Privileged:    raw.HostConfig.Privileged,
		User:          raw.Config.User,
		Hostname:      raw.Config.Hostname,
		Domainname:    raw.Config.Domainname,
		Labels:        raw.Config.Labels,
		CapAdd:        raw.HostConfig.CapAdd,
		CapDrop:       raw.HostConfig.CapDrop,
		DNS:           raw.HostConfig.DNS,
		DNSSearch:     raw.HostConfig.DNSSearch,
		ExtraHosts:    raw.HostConfig.ExtraHosts,
		LogConfig:     logConfig,
		PIDMode:       string(raw.HostConfig.PidMode),
		IPCMode:       string(raw.HostConfig.IpcMode),
		SecurityOpt:   raw.HostConfig.SecurityOpt,
		Tmpfs:         raw.HostConfig.Tmpfs,
		Sysctls:       raw.HostConfig.Sysctls,
		TTY:           raw.Config.Tty,
		OpenStdin:     raw.Config.OpenStdin,
		Healthcheck:   healthcheck,
		NetworkConfig: networkConfig,
	}

	return config, nil
}

func (c *Client) RecreateContainer(ctx context.Context, config *model.ContainerRecreateConfig) (string, error) {
	tmpName := config.Name + "-updating"
	wasRunning := config.State == "running"

	if wasRunning {
		if err := c.cli.ContainerStop(ctx, config.Name, container.StopOptions{}); err != nil {
			return "", fmt.Errorf("stop container: %w", err)
		}
	}

	envVars := make([]string, 0, len(config.EnvVars))
	for _, e := range config.EnvVars {
		envVars = append(envVars, e)
	}

	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			Memory:    config.Memory,
			CPUQuota:  config.CPUQuota,
			CPUPeriod: config.CPUPeriod,
			CPUShares: config.CPUShares,
		},
		Privileged: config.Privileged,
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyMode(config.RestartPolicy),
		},
		LogConfig: container.LogConfig{
			Type:   config.LogConfig.Type,
			Config: config.LogConfig.Config,
		},
		CapAdd:      config.CapAdd,
		CapDrop:     config.CapDrop,
		Tmpfs:       config.Tmpfs,
		DNS:         config.DNS,
		DNSSearch:   config.DNSSearch,
		ExtraHosts:  config.ExtraHosts,
		PidMode:     container.PidMode(config.PIDMode),
		IpcMode:     container.IpcMode(config.IPCMode),
		SecurityOpt: config.SecurityOpt,
		Sysctls:     config.Sysctls,
	}

	if len(config.Mounts) > 0 {
		binds := make([]string, 0)
		for _, m := range config.Mounts {
			bind := m.Source + ":" + m.Destination
			if m.ReadOnly {
				bind += ":ro"
			}
			binds = append(binds, bind)
		}
		hostConfig.Binds = binds
	}

	if len(config.PortBindings) > 0 {
		portBindings := make(map[nat.Port][]nat.PortBinding)
		for containerPort, bindings := range config.PortBindings {
			natPort := nat.Port(containerPort)
			for _, b := range bindings {
				portBindings[natPort] = append(portBindings[natPort], nat.PortBinding{
					HostIP:   b.HostIP,
					HostPort: b.HostPort,
				})
			}
		}
		hostConfig.PortBindings = portBindings
	}

	networkingConfig := &network.NetworkingConfig{}
	if config.NetworkMode != "" && config.NetworkMode != "default" {
		epSettings := &network.EndpointSettings{}
		if config.NetworkConfig != nil {
			epSettings.Aliases = config.NetworkConfig.Aliases
			if config.NetworkConfig.IPv4 != "" || config.NetworkConfig.IPv6 != "" {
				epSettings.IPAMConfig = &network.EndpointIPAMConfig{
					IPv4Address: config.NetworkConfig.IPv4,
					IPv6Address: config.NetworkConfig.IPv6,
				}
			}
		}
		networkingConfig = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				config.NetworkMode: epSettings,
			},
		}
	}

	containerConfig := &container.Config{
		Image:        config.Image,
		Env:          envVars,
		Cmd:          config.Cmd,
		Entrypoint:   config.Entrypoint,
		WorkingDir:   config.WorkingDir,
		User:         config.User,
		Hostname:     config.Hostname,
		Domainname:   config.Domainname,
		Labels:       config.Labels,
		Tty:          config.TTY,
		OpenStdin:    config.OpenStdin,
		ExposedPorts: getExposedPorts(config.PortBindings),
	}

	if config.Healthcheck != nil {
		containerConfig.Healthcheck = &container.HealthConfig{
			Test:    config.Healthcheck.Test,
			Retries: int(config.Healthcheck.Retries),
		}
		if config.Healthcheck.Interval != 0 {
			containerConfig.Healthcheck.Interval = time.Duration(config.Healthcheck.Interval) * time.Millisecond
		}
		if config.Healthcheck.Timeout != 0 {
			containerConfig.Healthcheck.Timeout = time.Duration(config.Healthcheck.Timeout) * time.Millisecond
		}
		if config.Healthcheck.StartPeriod != 0 {
			containerConfig.Healthcheck.StartPeriod = time.Duration(config.Healthcheck.StartPeriod) * time.Millisecond
		}
	}

	resp, err := c.cli.ContainerCreate(ctx, containerConfig, hostConfig, networkingConfig, nil, tmpName)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	if wasRunning || config.State == "paused" {
		if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			c.cli.ContainerRemove(ctx, tmpName, container.RemoveOptions{Force: true})
			return "", fmt.Errorf("start container: %w", err)
		}
	}

	if config.State == "paused" {
		if err := c.cli.ContainerUnpause(ctx, resp.ID); err != nil {
			c.cli.ContainerRemove(ctx, tmpName, container.RemoveOptions{Force: true})
			return "", fmt.Errorf("unpause container: %w", err)
		}
	}

	if err := c.cli.ContainerRemove(ctx, config.Name, container.RemoveOptions{Force: true}); err != nil {
		c.cli.ContainerRemove(ctx, tmpName, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("remove old container: %w", err)
	}

	if err := c.cli.ContainerRename(ctx, resp.ID, config.Name); err != nil {
		return "", fmt.Errorf("rename container: %w", err)
	}

	return resp.ID[:12], nil
}

func (c *Client) PullImage(ctx context.Context, imageName string, progressFn func(string, string)) (upToDate bool, err error) {
	digestBefore := c.getImageDigest(ctx, imageName)

	pullReader, err := c.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return false, fmt.Errorf("pull image %s: %w", imageName, err)
	}
	defer pullReader.Close()

	type pullStatus struct {
		Status         string `json:"status"`
		Progress       string `json:"progress"`
		ID             string `json:"id"`
		ProgressDetail struct {
			Current int64 `json:"current"`
			Total   int64 `json:"total"`
		} `json:"progressDetail"`
	}

	decoder := json.NewDecoder(pullReader)
	for {
		var ps pullStatus
		if err := decoder.Decode(&ps); err != nil {
			break
		}
		if progressFn != nil {
			msg := ps.Status
			if ps.ID != "" {
				msg = ps.ID + ": " + msg
			}
			if ps.Progress != "" {
				msg = msg + " " + ps.Progress
			}
			progressFn("pulling", msg)
		}
	}

	digestAfter := c.getImageDigest(ctx, imageName)
	upToDate = digestBefore != "" && digestBefore == digestAfter

	return upToDate, nil
}

func (c *Client) getImageDigest(ctx context.Context, imageName string) string {
	img, _, err := c.cli.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		return ""
	}
	return img.ID
}

func getExposedPorts(portBindings map[string][]model.PortBinding) map[nat.Port]struct{} {
	exposedPorts := make(map[nat.Port]struct{})
	for port := range portBindings {
		exposedPorts[nat.Port(port)] = struct{}{}
	}
	return exposedPorts
}
