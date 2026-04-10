package docker

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"

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
