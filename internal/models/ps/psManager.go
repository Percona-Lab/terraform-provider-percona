package ps

import (
	"encoding/base64"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	"strings"
	"terraform-percona/internal/models/ps/setup"
	"terraform-percona/internal/service"
)

const (
	RootPassword    = "password"
	ReplicaPassword = "replica_password"
)

func Create(cloud service.Cloud, resourceId string, size int64, pass, replicaPass string) error {
	instances, err := cloud.CreateInstances(resourceId, size, getBase64UserData())
	if err != nil {
		return errors.Wrap(err, "create instances")
	}
	binlogName, binlogPos := "", ""
	for i, instance := range instances {
		_, err = cloud.RunCommand(resourceId, instance, setup.Configure(pass))
		if err != nil {
			return errors.Wrap(err, "run command")
		}
		if len(instances) > 1 {
			_, err = cloud.RunCommand(resourceId, instance, setup.SetupReplication(i+1, instances[0].PrivateIpAddress, pass, replicaPass, binlogName, binlogPos))
			if err != nil {
				return errors.Wrap(err, "setup replication")
			}
		}
		_, err = cloud.RunCommand(resourceId, instance, setup.Start())
		if err != nil {
			return errors.Wrap(err, "run command")
		}
		if len(instances) > 1 {
			binlogName, binlogPos, err = currentBinlogAndPosition(resourceId, cloud, instance, pass)
			if err != nil {
				return errors.Wrap(err, "get binlog name and position")
			}
		}
	}
	return nil
}

func getBase64UserData() *string {
	return aws.String(base64.StdEncoding.EncodeToString([]byte(setup.Initial())))
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
