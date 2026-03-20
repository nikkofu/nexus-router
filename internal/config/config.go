package config

type Config struct {
	Server    ServerConfig     `yaml:"server"`
	Auth      AuthConfig       `yaml:"auth"`
	Models    []ModelConfig    `yaml:"models"`
	Providers []ProviderConfig `yaml:"providers"`
	Routing   RoutingConfig    `yaml:"routing"`
	Health    HealthConfig     `yaml:"health"`
	Breaker   BreakerConfig    `yaml:"breaker"`
	Limits    LimitsConfig     `yaml:"limits"`
}

type ServerConfig struct {
	ListenAddr      string    `yaml:"listen_addr"`
	AdminListenAddr string    `yaml:"admin_listen_addr"`
	TLS             TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	Mode     string `yaml:"mode"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type AuthConfig struct {
	ClientKeys []ClientKeyConfig `yaml:"client_keys"`
}

type ClientKeyConfig struct {
	ID                   string   `yaml:"id"`
	Secret               string   `yaml:"secret"`
	Active               bool     `yaml:"active"`
	AllowedModelPatterns []string `yaml:"allowed_model_patterns"`
	AllowStreaming       bool     `yaml:"allow_streaming"`
	AllowTools           bool     `yaml:"allow_tools"`
	AllowVision          bool     `yaml:"allow_vision"`
	AllowStructured      bool     `yaml:"allow_structured"`
}

type ModelConfig struct {
	Pattern    string `yaml:"pattern"`
	RouteGroup string `yaml:"route_group"`
}

type ProviderConfig struct {
	Name      string `yaml:"name"`
	Provider  string `yaml:"provider"`
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
}

type RoutingConfig struct {
	RouteGroups []RouteGroupConfig `yaml:"route_groups"`
}

type RouteGroupConfig struct {
	Name      string   `yaml:"name"`
	Primary   string   `yaml:"primary"`
	Fallbacks []string `yaml:"fallbacks"`
}

type HealthConfig struct {
	ProbeInterval string `yaml:"probe_interval"`
	ProbeTimeout  string `yaml:"probe_timeout"`
}

type BreakerConfig struct {
	FailureThreshold int    `yaml:"failure_threshold"`
	OpenInterval     string `yaml:"open_interval"`
}

type LimitsConfig struct {
	MaxRequestBytes int64 `yaml:"max_request_bytes"`
}
