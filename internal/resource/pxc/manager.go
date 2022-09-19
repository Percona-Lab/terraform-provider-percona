package pxc

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"terraform-percona/internal/cloud"
	"terraform-percona/internal/resource/pxc/cmd"
	"terraform-percona/internal/utils"
)

func Create(ctx context.Context, cloud cloud.Cloud, resourceId, password string, size int64, cfgPath, version string) ([]cloud.Instance, error) {
	tflog.Info(ctx, "Creating instances")
	instances, err := cloud.CreateInstances(ctx, resourceId, size)
	if err != nil {
		return nil, errors.Wrap(err, "create instances")
	}
	clusterAddresses := make([]string, 0, len(instances))
	for _, instance := range instances {
		clusterAddresses = append(clusterAddresses, instance.PrivateIpAddress)
	}
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(len(instances))
	for _, instance := range instances {
		instance := instance
		g.Go(func() error {
			_, err := cloud.RunCommand(gCtx, resourceId, instance, cmd.Initial())
			if err != nil {
				return errors.Wrap(err, "run command pxc initial")
			}
			_, err = cloud.RunCommand(gCtx, resourceId, instance, cmd.Configure(password))
			if err != nil {
				return errors.Wrap(err, "run command pxc configure")
			}
			availableVersions, err := versionList(gCtx, resourceId, cloud, instance)
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
			_, err = cloud.RunCommand(gCtx, resourceId, instance, cmd.InstallPerconaXtraDBCluster(clusterAddresses, version))
			if err != nil {
				return errors.Wrap(err, "install percona server")
			}
			if cfgPath != "" {
				if err = cloud.SendFile(gCtx, resourceId, cfgPath, "/etc/mysql/mysql.conf.d/custom.cnf", instance); err != nil {
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
		_, err = cloud.RunCommand(ctx, resourceId, instance, cmd.Start(i == 0))
		if err != nil {
			return nil, errors.Wrap(err, "run command pxc start")
		}
	}

	if len(instances) > 1 {
		if _, err = cloud.RunCommand(ctx, resourceId, instances[0], cmd.Stop(true)); err != nil {
			return nil, errors.Wrap(err, "run command bootstrap stop")
		}
		if _, err = cloud.RunCommand(ctx, resourceId, instances[0], cmd.Start(false)); err != nil {
			return nil, errors.Wrap(err, "run command first node pxc start")
		}
	}
	return instances, nil
}

func versionList(ctx context.Context, resourceId string, cloud cloud.Cloud, instance cloud.Instance) ([]string, error) {
	out, err := cloud.RunCommand(ctx, resourceId, instance, cmd.RetrieveVersions())
	if err != nil {
		return nil, errors.Wrap(err, "retrieve versions")
	}
	versions := strings.Split(out, "\n")
	if len(versions) == 0 {
		return nil, errors.New("no available versions")
	}
	return versions, nil
}
