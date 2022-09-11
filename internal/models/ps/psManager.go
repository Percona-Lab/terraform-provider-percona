package ps

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"terraform-percona/internal/cloud"
	"terraform-percona/internal/models/ps/setup"
	"terraform-percona/internal/service"
	"terraform-percona/internal/utils"
)

const (
	RootPassword    = "password"
	ReplicaPassword = "replica_password"
	MyRocksInstall  = "myrocks_install"
)

func Create(ctx context.Context, cloud cloud.Cloud, resourceId string, size int64, pass, replicaPass, cfgPath, version string, installMyRocks bool) ([]cloud.Instance, error) {
	tflog.Info(ctx, "Creating instances")
	instances, err := cloud.CreateInstances(ctx, resourceId, size)
	if err != nil {
		return nil, errors.Wrap(err, "create instances")
	}
	binlogName, binlogPos := "", ""
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(len(instances))
	for _, instance := range instances {
		instance := instance
		g.Go(func() error {
			_, err := cloud.RunCommand(gCtx, resourceId, instance, setup.Initial())
			if err != nil {
				return errors.Wrap(err, "run command")
			}
			_, err = cloud.RunCommand(gCtx, resourceId, instance, setup.Configure(pass))
			if err != nil {
				return errors.Wrap(err, "run command")
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
			tflog.Info(gCtx, "Installing Percona Server", map[string]interface{}{
				service.LogArgVersion:    version,
				service.LogArgInstanceIP: instance.PublicIpAddress,
			})
			_, err = cloud.RunCommand(gCtx, resourceId, instance, setup.InstallPerconaServer(pass, version))
			if err != nil {
				return errors.Wrap(err, "install percona server")
			}
			if cfgPath != "" {
				if err = cloud.SendFile(gCtx, resourceId, cfgPath, "/etc/mysql/mysql.conf.d/custom.cnf", instance); err != nil {
					return errors.Wrap(err, "failed to send config file")
				}
			}
			if installMyRocks {
				_, err = cloud.RunCommand(gCtx, resourceId, instance, setup.InstallMyRocks(pass, version))
				if err != nil {
					return errors.Wrap(err, "install myrocks")
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
		if len(instances) > 1 {
			_, err = cloud.RunCommand(ctx, resourceId, instance, setup.SetupReplication(i+1, instances[0].PrivateIpAddress, pass, replicaPass))
			if err != nil {
				return nil, errors.Wrap(err, "setup replication")
			}
		}
		_, err = cloud.RunCommand(ctx, resourceId, instance, setup.Restart())
		if err != nil {
			return nil, errors.Wrap(err, "run command")
		}
		if i != 0 {
			if _, err := cloud.RunCommand(ctx, resourceId, instance, setup.StartReplica(pass, i+1, instances[0].PrivateIpAddress, replicaPass, binlogName, binlogPos)); err != nil {
				return nil, errors.Wrap(err, "start replica")
			}
		}
		if len(instances) > 1 {
			binlogName, binlogPos, err = currentBinlogAndPosition(ctx, resourceId, cloud, instance, pass)
			if err != nil {
				return nil, errors.Wrap(err, "get binlog name and position")
			}
		}
	}
	return instances, nil
}

func currentBinlogAndPosition(ctx context.Context, resourceId string, cloud cloud.Cloud, instance cloud.Instance, pass string) (string, string, error) {
	out, err := cloud.RunCommand(ctx, resourceId, instance, setup.ShowMasterStatus(pass))
	if err != nil {
		return "", "", errors.Wrap(err, "run command")
	}
	name := ""
	pos := ""
	for _, line := range strings.Split(out, "\t") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if name == "" {
			name = line
			continue
		}
		pos = line
	}
	if name == "" || pos == "" {
		return "", "", errors.New("binlog name or position is empty")
	}
	return name, pos, nil
}

func versionList(ctx context.Context, resourceId string, cloud cloud.Cloud, instance cloud.Instance) ([]string, error) {
	out, err := cloud.RunCommand(ctx, resourceId, instance, setup.RetrieveVersions())
	if err != nil {
		return nil, errors.Wrap(err, "retrieve versions")
	}
	versions := strings.Split(out, "\n")
	if len(versions) == 0 {
		return nil, errors.New("no available versions")
	}
	return versions, nil
}
