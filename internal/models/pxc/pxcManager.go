package pxc

import (
	"encoding/base64"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	"terraform-percona/internal/models/pxc/setup"
	"terraform-percona/internal/service"
)

const MySQLPassword = "password"

func Create(cloud service.Cloud, resourceId, password string, size int64) error {
	instances, err := cloud.CreateInstances(resourceId, size, getBase64UserData())
	if err != nil {
		return errors.Wrap(err, "create instances")
	}
	clusterAddresses := make([]string, 0, len(instances))
	for _, instance := range instances {
		clusterAddresses = append(clusterAddresses, instance.PrivateIpAddress)
	}
	for i, instance := range instances {
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

func getBase64UserData() *string {
	//TODO add manager validation
	return aws.String(base64.StdEncoding.EncodeToString([]byte(setup.Initial())))
}
