package pxc

import (
	"context"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"strings"
	"terraform-percona/internal/models/pxc/setup"
	"terraform-percona/internal/service"
	"terraform-percona/internal/utils"
)

const MySQLPassword = "password"

func Create(ctx context.Context, cloud service.Cloud, resourceId, password string, size int64, cfgPath, version string) ([]service.Instance, error) {
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
			_, err = cloud.RunCommand(resourceId, instance, setup.Configure(password))
			if err != nil {
				return errors.Wrap(err, "run command pxc configure")
			}
			availableVersions, err := versionList(resourceId, cloud, instance)
			if err != nil {
				return errors.Wrap(err, "retrieve versions")
			}
			if version != "" {
				fullVersion := utils.SelectVersion(availableVersions, version)
				if fullVersion == "" {
					return errors.Errorf("version not found, available versions: %v", availableVersions)
				}
				version = fullVersion
			} else {
				version = availableVersions[0]
			}
			_, err = cloud.RunCommand(resourceId, instance, setup.InstallPerconaXtraDBCluster(clusterAddresses, version))
			if err != nil {
				return errors.Wrap(err, "install percona server")
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

	if len(instances) > 1 {
		if _, err = cloud.RunCommand(resourceId, instances[0], setup.Stop(true)); err != nil {
			return nil, errors.Wrap(err, "run command bootstrap stop")
		}
		if _, err = cloud.RunCommand(resourceId, instances[0], setup.Start(false)); err != nil {
			return nil, errors.Wrap(err, "run command first node pxc start")
		}
	}
	return instances, nil
}

func versionList(resourceId string, cloud service.Cloud, instance service.Instance) ([]string, error) {
	out, err := cloud.RunCommand(resourceId, instance, setup.RetrieveVersions())
	if err != nil {
		return nil, errors.Wrap(err, "retrieve versions")
	}
	versions := strings.Split(out, "\n")
	if len(versions) == 0 {
		return nil, errors.New("no available versions")
	}
	return versions, nil
}
