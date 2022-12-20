package cloud

import (
	"context"
	"io"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	AllAddressesCidrBlock = "0.0.0.0/0"
	DefaultVpcCidrBlock   = "10.0.0.0/16"
)

type Cloud interface {
	Configure(ctx context.Context, resourceID string, data *schema.ResourceData) error
	CreateInfrastructure(ctx context.Context, resourceID string) error
	DeleteInfrastructure(ctx context.Context, resourceID string) error
	RunCommand(ctx context.Context, resourceID string, instance Instance, cmd string) (string, error)
	SendFile(ctx context.Context, resourceID string, instance Instance, file io.Reader, remotePath string) error
	EditFile(ctx context.Context, resourceID string, instance Instance, path string, editFunc func(io.ReadWriteSeeker) error) error
	CreateInstances(ctx context.Context, resourceID string, size int64, labels map[string]string) ([]Instance, error)
	ListInstances(ctx context.Context, resourceID string, labels map[string]string) ([]Instance, error)
	Metadata() Metadata
	Credentials() (Credentials, error)
}

type Instance struct {
	PublicIpAddress  string
	PrivateIpAddress string
}

type Metadata struct {
	DisableTelemetry      bool
	IgnoreErrorsOnDestroy bool
}

type Credentials struct {
	AccessKey string
	SecretKey string
}
