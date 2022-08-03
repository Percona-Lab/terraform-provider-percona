package pxc

import (
	"context"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"terraform-percona/internal/models/pxc/setup"
	"terraform-percona/internal/service"
)

const MySQLPassword = "password"

func Create(ctx context.Context, cloud service.Cloud, resourceId, password string, size int64, cfgPath string) ([]service.Instance, error) {
	tflog.Info(ctx, "Creating instances")
	instances, err := cloud.CreateInstances(resourceId, size)
	if err != nil {
		return nil, errors.Wrap(err, "create instances")
	}
	clusterAddresses := make([]string, 0, len(instances))
	for _, instance := range instances {
		clusterAddresses = append(clusterAddresses, instance.PrivateIpAddress)
	}
	g := new(errgroup.Group)
	g.SetLimit(len(instances))
	for _, instance := range instances {
		instance := instance
		g.Go(func() error {
			_, err := cloud.RunCommand(resourceId, instance, setup.Initial())
			if err != nil {
				return errors.Wrap(err, "run command pxc initial")
			}
			_, err = cloud.RunCommand(resourceId, instance, setup.Configure(clusterAddresses, password))
			if err != nil {
				return errors.Wrap(err, "run command pxc configure")
			}
			if cfgPath != "" {
				if err = cloud.SendFile(resourceId, cfgPath, "/etc/mysql/mysql.conf.d/custom.cnf", instance); err != nil {
					return errors.Wrap(err, "failed to send config file")
				}
			}
			return nil
		})
	}
	tflog.Info(ctx, "Configuring instances")
	if err = g.Wait(); err != nil {
		return nil, errors.Wrap(err, "configure instances")
	}
	tflog.Info(ctx, "Starting instances")
	for i, instance := range instances {
		_, err = cloud.RunCommand(resourceId, instance, setup.Start(i == 0))
		if err != nil {
			return nil, errors.Wrap(err, "run command pxc start")
		}
	}
	return instances, nil
}
