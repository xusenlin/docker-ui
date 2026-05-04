package model

// ============================================================================
// 此文件包含 RecreateContainer 流程所需的全部模型定义。
//
// 如果你已有 ContainerRecreateConfig / MountConfig / PortBinding /
// LogConfig / HealthcheckConfig 等定义，请用本文件中的版本【整体替换】，
// 而不是同时存在两份（否则会重复声明编译报错）。
//
// 主要变更：
//   1. 新增 NetworkEndpoint 类型，ContainerRecreateConfig 用
//      []NetworkEndpoint 替代原来的 *NetworkConfig（多网络支持）
//   2. MountConfig 增加 Type 和 Name 字段（区分 bind / volume / tmpfs）
//   3. ContainerRecreateConfig 增加多个之前丢失的字段：
//      Devices / StopSignal / StopTimeout / Init / Runtime /
//      ReadonlyRootfs / GroupAdd / OomScoreAdj / ImageEnv /
//      ImageCmd / ImageEntrypoint
// ============================================================================

// NetworkEndpoint 描述容器在某个网络上的端点配置。
// 一个容器可以同时连接到多个网络，每个网络对应一个 NetworkEndpoint。
type NetworkEndpoint struct {
	Name      string   // 网络名称（如 "web"、"bridge"）
	NetworkID string   // 网络 ID（可选，重连时优先使用 Name）
	Aliases   []string // 用户定义的网络别名（已过滤 Docker 自动生成的别名）
	IPv4      string   // 用户指定的固定 IPv4（可空）
	IPv6      string   // 用户指定的固定 IPv6（可空）
}

// PortBinding 端口绑定配置。
type PortBinding struct {
	HostIP   string
	HostPort string
}

// MountConfig 挂载配置。Type 字段用于区分 bind / volume / tmpfs。
type MountConfig struct {
	Type        string // "bind" | "volume" | "tmpfs"
	Source      string // bind: 宿主机路径；volume: 宿主机内部路径（不直接使用）
	Name        string // volume: 卷名（这是重建时实际使用的字段）
	Destination string
	Mode        string // 原始 mode 字符串（包含 ro/rw、SELinux 标签等）
	ReadOnly    bool
}

// LogConfig 日志驱动配置。
type LogConfig struct {
	Type   string
	Config map[string]string
}

// HealthcheckConfig 健康检查配置。Interval/Timeout/StartPeriod 单位为毫秒。
type HealthcheckConfig struct {
	Test        []string
	Interval    int64
	Timeout     int64
	StartPeriod int64
	Retries     int64
}

// DeviceMapping 设备映射（--device 参数）。
type DeviceMapping struct {
	PathOnHost        string
	PathInContainer   string
	CgroupPermissions string
}

// ContainerRecreateConfig 是重建容器所需的完整配置快照。
// 由 GetRecreateConfig 从 docker inspect 数据生成，由 RecreateContainer 消费。
type ContainerRecreateConfig struct {
	// 基础
	Name  string
	Image string // 镜像引用（如 "nginx:latest"），用于拉取和创建新容器
	State string // 原容器状态："running" | "paused" | "exited" | ...

	// Config 字段
	EnvVars     []string // 已过滤镜像默认值后的用户环境变量
	Cmd         []string // 仅当用户显式覆盖了镜像默认 CMD 时才非空
	Entrypoint  []string // 仅当用户显式覆盖了镜像默认 ENTRYPOINT 时才非空
	WorkingDir  string
	User        string
	Hostname    string
	Domainname  string
	Labels      map[string]string
	TTY         bool
	OpenStdin   bool
	StopSignal  string // 优雅关闭信号（如 SIGTERM、SIGINT）
	StopTimeout *int   // 停止超时（秒），nil 使用 Docker 默认

	// HostConfig 字段
	NetworkMode    string
	PortBindings   map[string][]PortBinding
	Mounts         []MountConfig
	RestartPolicy  string
	Memory         int64
	CPUQuota       int64
	CPUPeriod      int64
	CPUShares      int64
	Privileged     bool
	CapAdd         []string
	CapDrop        []string
	DNS            []string
	DNSSearch      []string
	ExtraHosts     []string
	LogConfig      LogConfig
	PIDMode        string
	IPCMode        string
	SecurityOpt    []string
	Tmpfs          map[string]string
	Sysctls        map[string]string
	Devices        []DeviceMapping // --device
	Init           *bool           // --init
	Runtime        string          // --runtime（如 "nvidia"、"runsc"）
	ReadonlyRootfs bool            // --read-only
	GroupAdd       []string        // --group-add
	OomScoreAdj    int             // --oom-score-adj

	// 网络（多网络支持）
	NetworkEndpoints []NetworkEndpoint

	// 健康检查
	Healthcheck *HealthcheckConfig

	// 镜像默认值快照（用于排查、对比；运行时不直接使用）
	ImageEnv        []string
	ImageCmd        []string
	ImageEntrypoint []string
}
