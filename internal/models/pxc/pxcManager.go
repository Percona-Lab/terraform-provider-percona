package pxc

import (
	"github.com/pkg/errors"
	"terraform-percona/internal/models/pxc/setup"
	"terraform-percona/internal/service"
)

const MySQLPassword = "password"

func Create(cloud service.Cloud, resourceId, password string, size int64) error {
	instances, err := cloud.CreateInstances(resourceId, size)
	if err != nil {
		return errors.Wrap(err, "create instances")
	}
	clusterAddresses := make([]string, 0, len(instances))
	for _, instance := range instances {
		clusterAddresses = append(clusterAddresses, instance.PrivateIpAddress)
	}
	for i, instance := range instances {
		_, err = cloud.RunCommand(resourceId, instance, setup.Initial())
		if err != nil {
			return errors.Wrap(err, "run command pxc initial")
		}
		_, err = cloud.RunCommand(resourceId, instance, setup.Configure(clusterAddresses, password))
		if err != nil {
			return errors.Wrap(err, "run command pxc configure")
		}
		_, err = cloud.RunCommand(resourceId, instance, setup.Start(i == 0))
		if err != nil {
			return errors.Wrap(err, "run command pxc start")
		}
	}
	return nil
}
