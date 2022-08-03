package ps

import (
	"context"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"strings"
	"terraform-percona/internal/models/ps/setup"
	"terraform-percona/internal/service"
)

const (
	RootPassword    = "password"
	ReplicaPassword = "replica_password"
)

func Create(ctx context.Context, cloud service.Cloud, resourceId string, size int64, pass, replicaPass, cfgPath string) ([]service.Instance, error) {
	tflog.Info(ctx, "Creating instances")
	instances, err := cloud.CreateInstances(resourceId, size)
	if err != nil {
		return nil, errors.Wrap(err, "create instances")
	}
	binlogName, binlogPos := "", ""
	g := new(errgroup.Group)
	g.SetLimit(len(instances))
	for _, instance := range instances {
		instance := instance
		g.Go(func() error {
			_, err := cloud.RunCommand(resourceId, instance, setup.Initial())
			if err != nil {
				return errors.Wrap(err, "run command")
			}
			_, err = cloud.RunCommand(resourceId, instance, setup.Configure(pass))
			if err != nil {
				return errors.Wrap(err, "run command")
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
		if len(instances) > 1 {
			_, err = cloud.RunCommand(resourceId, instance, setup.SetupReplication(i+1, instances[0].PrivateIpAddress, pass, replicaPass, binlogName, binlogPos))
			if err != nil {
				return nil, errors.Wrap(err, "setup replication")
			}
		}
		_, err = cloud.RunCommand(resourceId, instance, setup.Start())
		if err != nil {
			return nil, errors.Wrap(err, "run command")
		}
		if len(instances) > 1 {
			binlogName, binlogPos, err = currentBinlogAndPosition(resourceId, cloud, instance, pass)
			if err != nil {
				return nil, errors.Wrap(err, "get binlog name and position")
			}
		}
	}
	return instances, nil
}

func currentBinlogAndPosition(resourceId string, cloud service.Cloud, instance service.Instance, pass string) (string, string, error) {
	out, err := cloud.RunCommand(resourceId, instance, setup.ShowMasterStatus(pass))
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
