package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/go-connections/nat"

	"docker-ui/internal/model"
)

// updateLocks 保证同一个容器不会被并发重建。
// key: 容器名称，value: *sync.Mutex
var updateLocks sync.Map

// acquireUpdateLock 尝试获取容器的更新锁。
// 若当前已有更新在进行中，返回 ok=false。
func acquireUpdateLock(name string) (release func(), ok bool) {
	muxAny, _ := updateLocks.LoadOrStore(name, &sync.Mutex{})
	mux := muxAny.(*sync.Mutex)
	if !mux.TryLock() {
		return nil, false
	}
	return mux.Unlock, true
}

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

	name := strings.TrimPrefix(raw.Name, "/")

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

// GetContainerImageID 返回容器创建时绑定的镜像 SHA。
// 用于 PullImage 之后判断是否真的有新版本可用。
func (c *Client) GetContainerImageID(ctx context.Context, id string) (string, error) {
	raw, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		return "", fmt.Errorf("inspect container %s: %w", id, err)
	}
	return raw.Image, nil
}

// GetRecreateConfig 从 docker inspect 数据收集重建容器所需的完整配置。
// 关键改进：
//   - 收集所有网络（不再只保留第一个）
//   - 过滤镜像默认 Env / Cmd / Entrypoint，只保留用户自定义的部分
//   - 区分 bind / volume / tmpfs 挂载类型
//   - 使用 HostConfig.PortBindings（创建时的配置）而非 NetworkSettings.Ports（运行时映射）
//   - 收集 Devices、StopSignal、Init、Runtime 等之前遗漏的字段
func (c *Client) GetRecreateConfig(ctx context.Context, id string) (*model.ContainerRecreateConfig, error) {
	raw, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("inspect container %s: %w", id, err)
	}

	containerName := strings.TrimPrefix(raw.Name, "/")

	// 拿旧镜像的默认配置，用于过滤 Env/Cmd/Entrypoint
	var imageEnv []string
	var imageCmd, imageEntrypoint strslice.StrSlice
	if oldImg, _, err := c.cli.ImageInspectWithRaw(ctx, raw.Image); err == nil {
		if oldImg.Config != nil {
			imageEnv = oldImg.Config.Env
			imageCmd = oldImg.Config.Cmd
			imageEntrypoint = oldImg.Config.Entrypoint
		}
	}
	// 注意：如果 ImageInspect 失败（旧镜像已被删除），我们退化成不过滤——
	// 这种情况下重建容器仍可以工作，只是新镜像的默认 ENV 可能被旧值覆盖。

	// 过滤 Env：去掉镜像默认值，只保留用户自定义
	imageEnvSet := make(map[string]bool, len(imageEnv))
	for _, e := range imageEnv {
		imageEnvSet[e] = true
	}
	userEnv := make([]string, 0)
	for _, e := range raw.Config.Env {
		if !imageEnvSet[e] {
			userEnv = append(userEnv, e)
		}
	}

	// 过滤 Cmd：只在用户显式覆盖时保留
	var userCmd []string
	if !slicesEqual(raw.Config.Cmd, imageCmd) {
		userCmd = raw.Config.Cmd
	}

	// 过滤 Entrypoint
	var userEntrypoint []string
	if !slicesEqual(raw.Config.Entrypoint, imageEntrypoint) {
		userEntrypoint = raw.Config.Entrypoint
	}

	// 端口绑定：使用 HostConfig.PortBindings（容器创建时的配置）
	// 而不是 NetworkSettings.Ports（仅在运行时有值）
	portBindings := make(map[string][]model.PortBinding)
	for containerPort, bindings := range raw.HostConfig.PortBindings {
		for _, b := range bindings {
			portBindings[string(containerPort)] = append(portBindings[string(containerPort)], model.PortBinding{
				HostIP:   b.HostIP,
				HostPort: b.HostPort,
			})
		}
	}

	// 挂载：保留类型（bind / volume / tmpfs）和卷名
	mounts := make([]model.MountConfig, 0, len(raw.Mounts))
	for _, m := range raw.Mounts {
		mounts = append(mounts, model.MountConfig{
			Type:        string(m.Type),
			Source:      m.Source,
			Name:        m.Name,
			Destination: m.Destination,
			Mode:        m.Mode,
			ReadOnly:    !m.RW,
		})
	}

	// 网络：收集所有网络端点，不再只保留第一个
	endpoints := make([]model.NetworkEndpoint, 0, len(raw.NetworkSettings.Networks))
	for netName, ep := range raw.NetworkSettings.Networks {
		aliases := filterAutoAliases(ep.Aliases, raw.ID, containerName)
		nep := model.NetworkEndpoint{
			Name:      netName,
			NetworkID: ep.NetworkID,
			Aliases:   aliases,
		}
		if ep.IPAMConfig != nil {
			nep.IPv4 = ep.IPAMConfig.IPv4Address
			nep.IPv6 = ep.IPAMConfig.IPv6Address
		}
		endpoints = append(endpoints, nep)
	}
	// 排序保证多次调用结果稳定（map 遍历无序）
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].Name < endpoints[j].Name
	})

	// 设备映射
	devices := make([]model.DeviceMapping, 0, len(raw.HostConfig.Devices))
	for _, d := range raw.HostConfig.Devices {
		devices = append(devices, model.DeviceMapping{
			PathOnHost:        d.PathOnHost,
			PathInContainer:   d.PathInContainer,
			CgroupPermissions: d.CgroupPermissions,
		})
	}

	// 健康检查
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

	logConfig := model.LogConfig{
		Type:   raw.HostConfig.LogConfig.Type,
		Config: raw.HostConfig.LogConfig.Config,
	}

	config := &model.ContainerRecreateConfig{
		Name:  containerName,
		Image: raw.Config.Image,
		State: raw.State.Status,

		// Config 字段
		EnvVars:     userEnv,
		Cmd:         userCmd,
		Entrypoint:  userEntrypoint,
		WorkingDir:  raw.Config.WorkingDir,
		User:        raw.Config.User,
		Hostname:    raw.Config.Hostname,
		Domainname:  raw.Config.Domainname,
		Labels:      raw.Config.Labels,
		TTY:         raw.Config.Tty,
		OpenStdin:   raw.Config.OpenStdin,
		StopSignal:  raw.Config.StopSignal,
		StopTimeout: raw.Config.StopTimeout,

		// HostConfig 字段
		NetworkMode:    string(raw.HostConfig.NetworkMode),
		PortBindings:   portBindings,
		Mounts:         mounts,
		RestartPolicy:  string(raw.HostConfig.RestartPolicy.Name),
		Memory:         raw.HostConfig.Memory,
		CPUQuota:       raw.HostConfig.CPUQuota,
		CPUPeriod:      raw.HostConfig.CPUPeriod,
		CPUShares:      raw.HostConfig.CPUShares,
		Privileged:     raw.HostConfig.Privileged,
		CapAdd:         raw.HostConfig.CapAdd,
		CapDrop:        raw.HostConfig.CapDrop,
		DNS:            raw.HostConfig.DNS,
		DNSSearch:      raw.HostConfig.DNSSearch,
		ExtraHosts:     raw.HostConfig.ExtraHosts,
		LogConfig:      logConfig,
		PIDMode:        string(raw.HostConfig.PidMode),
		IPCMode:        string(raw.HostConfig.IpcMode),
		SecurityOpt:    raw.HostConfig.SecurityOpt,
		Tmpfs:          raw.HostConfig.Tmpfs,
		Sysctls:        raw.HostConfig.Sysctls,
		Devices:        devices,
		Init:           raw.HostConfig.Init,
		Runtime:        raw.HostConfig.Runtime,
		ReadonlyRootfs: raw.HostConfig.ReadonlyRootfs,
		GroupAdd:       raw.HostConfig.GroupAdd,
		OomScoreAdj:    raw.HostConfig.OomScoreAdj,

		// 网络（多网络）
		NetworkEndpoints: endpoints,

		// 健康检查
		Healthcheck: healthcheck,

		// 镜像默认值快照
		ImageEnv:        imageEnv,
		ImageCmd:        []string(imageCmd),
		ImageEntrypoint: []string(imageEntrypoint),
	}

	return config, nil
}

// filterAutoAliases 过滤掉 Docker 自动加的网络别名（短 ID、容器名）。
// 只保留用户通过 --network-alias 显式设置的别名。
func filterAutoAliases(aliases []string, containerID, containerName string) []string {
	shortID := containerID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	out := make([]string, 0, len(aliases))
	for _, a := range aliases {
		if a == shortID || a == containerName {
			continue
		}
		out = append(out, a)
	}
	return out
}

// slicesEqual 判断两个字符串切片是否完全相等。
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// isSpecialNetworkMode 判断 NetworkMode 是否是不需要 EndpointsConfig 的特殊模式。
func isSpecialNetworkMode(mode string) bool {
	return mode == "host" || mode == "none" || strings.HasPrefix(mode, "container:")
}

// RecreateContainer 用新镜像重建容器，尽可能保留所有原始运行参数。
//
// 流程：
//  1. 获取并发锁，避免同一容器被同时多次更新
//  2. 若原容器在 paused，先 unpause（否则 stop 不能正常工作）
//  3. 若原容器在运行，stop 它
//  4. 把原容器 rename 成 -bak（释放原名，便于回滚）
//  5. 用原名 create 新容器
//  6. 把所有"次要"网络 NetworkConnect 上去
//  7. 启动新容器（如果原本是 running 或 paused）
//  8. 重新 pause（如果原本是 paused）
//  9. 删除 -bak 旧容器
//
// 任何中间步骤失败都会回滚：删除新容器，把 -bak 重命名回原名，按原状态启动。
func (c *Client) RecreateContainer(ctx context.Context, config *model.ContainerRecreateConfig) (string, error) {
	release, ok := acquireUpdateLock(config.Name)
	if !ok {
		return "", fmt.Errorf("container %s is being updated by another request", config.Name)
	}
	defer release()

	name := config.Name
	backupName := name + "-bak"
	wasRunning := config.State == "running"
	wasPaused := config.State == "paused"

	// Step 1: 如果是 paused 状态，先 unpause（否则 stop 没用）
	if wasPaused {
		if err := c.cli.ContainerUnpause(ctx, name); err != nil {
			log.Printf("warn: unpause %s before recreate: %v", name, err)
			// 继续尝试，不致命
		}
	}

	// Step 2: 停止原容器
	if wasRunning || wasPaused {
		if err := c.cli.ContainerStop(ctx, name, container.StopOptions{}); err != nil {
			return "", fmt.Errorf("stop container: %w", err)
		}
	}

	// Step 3: 把原容器改名让出原名
	if err := c.cli.ContainerRename(ctx, name, backupName); err != nil {
		// 回滚：把原容器恢复到原状态
		if wasRunning || wasPaused {
			_ = c.cli.ContainerStart(ctx, name, container.StartOptions{})
			if wasPaused {
				_ = c.cli.ContainerPause(ctx, name)
			}
		}
		return "", fmt.Errorf("rename old container: %w", err)
	}

	// 失败回滚函数：清理已创建的新容器，把 backup 改回原名并恢复状态
	rollback := func(newID, reason string, cause error) error {
		if newID != "" {
			_ = c.cli.ContainerRemove(ctx, newID, container.RemoveOptions{Force: true, RemoveVolumes: false})
		}
		// 把 backup 改回原名
		if err := c.cli.ContainerRename(ctx, backupName, name); err != nil {
			return fmt.Errorf("%s: %w (rollback failed: rename back: %v)", reason, cause, err)
		}
		// 恢复状态
		if wasRunning || wasPaused {
			if err := c.cli.ContainerStart(ctx, name, container.StartOptions{}); err != nil {
				return fmt.Errorf("%s: %w (rollback failed: start: %v)", reason, cause, err)
			}
			if wasPaused {
				_ = c.cli.ContainerPause(ctx, name)
			}
		}
		return fmt.Errorf("%s, rolled back to old container: %w", reason, cause)
	}

	// Step 4: 构造 docker create 参数

	containerConfig := buildContainerConfig(config)
	hostConfig := buildHostConfig(config)
	networkingConfig, additionalEndpoints := buildNetworkingConfig(config)

	// Step 5: 创建新容器（用原名）
	resp, err := c.cli.ContainerCreate(ctx, containerConfig, hostConfig, networkingConfig, nil, name)
	if err != nil {
		return "", rollback("", "create new container", err)
	}

	// Step 6: 把次要网络连上
	for _, ep := range additionalEndpoints {
		netRef := ep.Name
		if netRef == "" && ep.NetworkID != "" {
			netRef = ep.NetworkID
		}
		epSettings := buildEndpointSettings(ep)
		if err := c.cli.NetworkConnect(ctx, netRef, resp.ID, epSettings); err != nil {
			return "", rollback(resp.ID, fmt.Sprintf("connect network %s", ep.Name), err)
		}
	}

	// Step 7: 启动新容器（如果原本在运行或 paused）
	if wasRunning || wasPaused {
		if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			return "", rollback(resp.ID, "start new container", err)
		}
	}

	// Step 8: 如果原状态是 paused，重新 pause
	if wasPaused {
		if err := c.cli.ContainerPause(ctx, resp.ID); err != nil {
			// 已经成功重建并启动了，不再回滚——只记 warning
			log.Printf("warn: re-pause %s after recreate: %v", name, err)
		}
	}

	// Step 9: 删除旧容器（backup）
	if err := c.cli.ContainerRemove(ctx, backupName, container.RemoveOptions{Force: true, RemoveVolumes: false}); err != nil {
		// 此时新容器已正常工作，旧容器删不掉只是脏数据，不算失败
		log.Printf("warn: remove backup container %s: %v", backupName, err)
	}

	return resp.ID[:12], nil
}

func buildContainerConfig(config *model.ContainerRecreateConfig) *container.Config {
	cfg := &container.Config{
		Image:        config.Image,
		Env:          config.EnvVars,
		WorkingDir:   config.WorkingDir,
		User:         config.User,
		Hostname:     config.Hostname,
		Domainname:   config.Domainname,
		Labels:       config.Labels,
		Tty:          config.TTY,
		OpenStdin:    config.OpenStdin,
		StopSignal:   config.StopSignal,
		StopTimeout:  config.StopTimeout,
		ExposedPorts: getExposedPorts(config.PortBindings),
	}

	// Cmd / Entrypoint：只在用户显式覆盖时设置；否则用 nil 让镜像默认值生效
	if len(config.Cmd) > 0 {
		cfg.Cmd = strslice.StrSlice(config.Cmd)
	}
	if len(config.Entrypoint) > 0 {
		cfg.Entrypoint = strslice.StrSlice(config.Entrypoint)
	}

	if config.Healthcheck != nil {
		cfg.Healthcheck = &container.HealthConfig{
			Test:    config.Healthcheck.Test,
			Retries: int(config.Healthcheck.Retries),
		}
		if config.Healthcheck.Interval != 0 {
			cfg.Healthcheck.Interval = time.Duration(config.Healthcheck.Interval) * time.Millisecond
		}
		if config.Healthcheck.Timeout != 0 {
			cfg.Healthcheck.Timeout = time.Duration(config.Healthcheck.Timeout) * time.Millisecond
		}
		if config.Healthcheck.StartPeriod != 0 {
			cfg.Healthcheck.StartPeriod = time.Duration(config.Healthcheck.StartPeriod) * time.Millisecond
		}
	}

	return cfg
}

func buildHostConfig(config *model.ContainerRecreateConfig) *container.HostConfig {
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
		CapAdd:         config.CapAdd,
		CapDrop:        config.CapDrop,
		Tmpfs:          config.Tmpfs,
		DNS:            config.DNS,
		DNSSearch:      config.DNSSearch,
		ExtraHosts:     config.ExtraHosts,
		PidMode:        container.PidMode(config.PIDMode),
		IpcMode:        container.IpcMode(config.IPCMode),
		SecurityOpt:    config.SecurityOpt,
		Sysctls:        config.Sysctls,
		NetworkMode:    container.NetworkMode(config.NetworkMode),
		Init:           config.Init,
		Runtime:        config.Runtime,
		ReadonlyRootfs: config.ReadonlyRootfs,
		GroupAdd:       config.GroupAdd,
		OomScoreAdj:    config.OomScoreAdj,
	}

	// 区分 bind / volume / tmpfs
	var binds []string
	var mountList []mount.Mount
	for _, m := range config.Mounts {
		switch strings.ToLower(m.Type) {
		case "volume":
			src := m.Name
			if src == "" {
				// 没有名字的卷，跳过（一般是匿名 VOLUME，重建后自动生成新的）
				continue
			}
			mountList = append(mountList, mount.Mount{
				Type:     mount.TypeVolume,
				Source:   src,
				Target:   m.Destination,
				ReadOnly: m.ReadOnly,
			})
		case "tmpfs":
			// tmpfs 已经通过 hostConfig.Tmpfs 处理，这里跳过避免重复
			continue
		default:
			// "bind" 或未知类型，按 bind mount 处理
			bind := m.Source + ":" + m.Destination
			if m.Mode != "" && m.Mode != "rw" {
				bind += ":" + m.Mode
			} else if m.ReadOnly {
				bind += ":ro"
			}
			binds = append(binds, bind)
		}
	}
	if len(binds) > 0 {
		hostConfig.Binds = binds
	}
	if len(mountList) > 0 {
		hostConfig.Mounts = mountList
	}

	// 端口绑定
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

	// 设备
	if len(config.Devices) > 0 {
		devices := make([]container.DeviceMapping, 0, len(config.Devices))
		for _, d := range config.Devices {
			devices = append(devices, container.DeviceMapping{
				PathOnHost:        d.PathOnHost,
				PathInContainer:   d.PathInContainer,
				CgroupPermissions: d.CgroupPermissions,
			})
		}
		hostConfig.Devices = devices
	}

	return hostConfig
}

// buildNetworkingConfig 决定 ContainerCreate 时使用哪个网络作为"主网络"，
// 并返回需要在创建后通过 NetworkConnect 加入的"次要网络"列表。
//
// Docker API 限制：ContainerCreate 一次最多只能加入一个网络。
// 多网络的容器必须 create 后 NetworkConnect 其余网络。
func buildNetworkingConfig(config *model.ContainerRecreateConfig) (*network.NetworkingConfig, []model.NetworkEndpoint) {
	// 特殊网络模式（host / none / container:xxx）：不需要 EndpointsConfig
	if isSpecialNetworkMode(config.NetworkMode) {
		return nil, nil
	}

	if len(config.NetworkEndpoints) == 0 {
		return nil, nil
	}

	// 选主网络：优先匹配 NetworkMode；否则选第一个
	primaryIdx := 0
	for i, ep := range config.NetworkEndpoints {
		if ep.Name == config.NetworkMode {
			primaryIdx = i
			break
		}
	}
	primary := config.NetworkEndpoints[primaryIdx]

	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			primary.Name: buildEndpointSettings(primary),
		},
	}

	// 收集次要网络
	var additional []model.NetworkEndpoint
	for i, ep := range config.NetworkEndpoints {
		if i != primaryIdx {
			additional = append(additional, ep)
		}
	}

	return netCfg, additional
}

func buildEndpointSettings(ep model.NetworkEndpoint) *network.EndpointSettings {
	s := &network.EndpointSettings{
		NetworkID: ep.NetworkID,
		Aliases:   ep.Aliases,
	}
	if ep.IPv4 != "" || ep.IPv6 != "" {
		s.IPAMConfig = &network.EndpointIPAMConfig{
			IPv4Address: ep.IPv4,
			IPv6Address: ep.IPv6,
		}
	}
	return s
}

// PullImage 拉取镜像，并通过对比 SHA 判断容器当前用的镜像是否已是最新。
//
// containerImageID 应当传容器创建时绑定的镜像 SHA（即 ContainerInspect.Image）。
// 如果传空字符串，永远返回 upToDate=false（即一定走重建流程）。
func (c *Client) PullImage(ctx context.Context, imageName, containerImageID string, progressFn func(string, string)) (upToDate bool, err error) {
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

	// 比较：容器当前绑定的镜像 SHA vs pull 后本地最新的 SHA
	latestSHA := c.getImageDigest(ctx, imageName)
	upToDate = containerImageID != "" && containerImageID == latestSHA

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
