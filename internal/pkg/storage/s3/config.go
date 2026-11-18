package s3

import "time"

// Config 是 S3 兼容对象存储（RustFS / MinIO / AWS S3）的连接参数。
// 由 etc/*.yaml 的 storage 段加载；密钥通过 GOIM_BIZ_STORAGE__SECRET_KEY 环境变量覆盖。
type Config struct {
	// Endpoint 是对象存储服务地址，例如 http://rustfs:9000。
	Endpoint string `mapstructure:"endpoint" yaml:"endpoint" json:"endpoint"`
	// PublicEndpoint 是客户端直传/下载使用的对外地址（公网域名或 CDN）；
	// 为空时回退 Endpoint。
	PublicEndpoint string `mapstructure:"public_endpoint" yaml:"publicEndpoint" json:"publicEndpoint"`
	// AccessKey 访问密钥 ID。
	AccessKey string `mapstructure:"access_key" yaml:"accessKey" json:"accessKey"`
	// SecretKey 访问密钥 Secret；只能通过环境变量注入，禁止写入仓库。
	SecretKey string `mapstructure:"secret_key" yaml:"secretKey" json:"secretKey"`
	// Region AWS 风格区域号，RustFS/MinIO 填任意非空值。
	Region string `mapstructure:"region" yaml:"region" json:"region"`
	// Bucket 对象存放的 bucket；为空表示未启用对象存储。
	Bucket string `mapstructure:"bucket" yaml:"bucket" json:"bucket"`
	// UsePathStyle RustFS/MinIO 需要 path-style（endpoint/bucket/key）。
	UsePathStyle bool `mapstructure:"use_path_style" yaml:"usePathStyle" json:"usePathStyle"`
	// PresignExpire 是预签名 URL 的默认有效期。
	PresignExpire time.Duration `mapstructure:"presign_expire" yaml:"presignExpire" json:"presignExpire"`
}

// Enabled 表示对象存储已配置（endpoint 与 bucket 同时存在）。
func (c *Config) Enabled() bool {
	return c != nil && c.Endpoint != "" && c.Bucket != ""
}

// publicBase 返回对外地址，未配置 PublicEndpoint 时回退 Endpoint。
func (c *Config) publicBase() string {
	if c.PublicEndpoint != "" {
		return c.PublicEndpoint
	}
	return c.Endpoint
}
