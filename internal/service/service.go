package service

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

const (
	ResourceIdLen = 20
)

type Instance struct {
	PublicIpAddress  string
	PrivateIpAddress string
}

type Cloud interface {
	Configure(resourceId string, data *schema.ResourceData) error
	CreateInfrastructure(resourceId string) error
	DeleteInfrastructure(resourceId string) error
	RunCommand(resourceId string, instance Instance, cmd string) (string, error)
	CreateInstances(resourceId string, size int64) ([]Instance, error)
}
