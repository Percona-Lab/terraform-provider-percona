package cloud

import (
	"context"
	"io"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type Cloud interface {
	Configure(ctx context.Context, resourceID string, data *schema.ResourceData) error
	CreateInfrastructure(ctx context.Context, resourceID string) error
	DeleteInfrastructure(ctx context.Context, resourceID string) error
	RunCommand(ctx context.Context, resourceID string, instance Instance, cmd string) (string, error)
	SendFile(ctx context.Context, resourceID string, instance Instance, filePath, remotePath string) error
	EditFile(ctx context.Context, resourceID string, instance Instance, path string, editFunc func(io.ReadWriteSeeker) error) error
	CreateInstances(ctx context.Context, resourceID string, size int64) ([]Instance, error)
	Metadata() Metadata
}

type Instance struct {
	PublicIpAddress  string
	PrivateIpAddress string
}

type Metadata struct {
	DisableTelemetry      bool
	IgnoreErrorsOnDestroy bool
}
