package cloud

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type Cloud interface {
	Configure(ctx context.Context, resourceId string, data *schema.ResourceData) error
	CreateInfrastructure(ctx context.Context, resourceId string) error
	DeleteInfrastructure(ctx context.Context, resourceId string) error
	RunCommand(ctx context.Context, resourceId string, instance Instance, cmd string) (string, error)
	SendFile(ctx context.Context, resourceId, filePath, remotePath string, instance Instance) error
	CreateInstances(ctx context.Context, resourceId string, size int64) ([]Instance, error)
}

type Instance struct {
	PublicIpAddress  string
	PrivateIpAddress string
}
