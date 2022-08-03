package aws

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"path"
	"path/filepath"
	"terraform-percona/internal/service"
	"terraform-percona/internal/utils"
)

type Cloud struct {
	Region  *string
	Profile *string

	client  *ec2.EC2
	session *session.Session

	configs map[string]*resourceConfig
}

type resourceConfig struct {
	keyPair         *string
	pathToKeyPair   *string
	securityGroupID *string
	subnetID        *string
	ami             *string
	instanceType    *string
	volumeSize      *int64
	volumeType      *string
}

func (cloud *Cloud) RunCommand(resourceId string, instance service.Instance, cmd string) (string, error) {
	sshKeyPath, err := cloud.keyPairPath(resourceId)
	if err != nil {
		return "", errors.Wrap(err, "get key pair path")
	}
	sshConfig, err := utils.SSHConfig("ubuntu", sshKeyPath)
	if err != nil {
		return "", errors.Wrap(err, "get ssh config")
	}
	return utils.RunCommand(cmd, instance.PublicIpAddress, sshConfig)
}

func (cloud *Cloud) SendFile(resourceId, filePath, remotePath string, instance service.Instance) error {
	sshKeyPath, err := cloud.keyPairPath(resourceId)
	if err != nil {
		return errors.Wrap(err, "get key pair path")
	}
	sshConfig, err := utils.SSHConfig("ubuntu", sshKeyPath)
	if err != nil {
		return errors.Wrap(err, "get ssh config")
	}
	return utils.SendFile(filePath, remotePath, instance.PublicIpAddress, sshConfig)
}

func (cloud *Cloud) CreateInstances(resourceId string, size int64) ([]service.Instance, error) {
	instanceIds := make([]*string, 0, size)
	cfg := cloud.configs[resourceId]
	for i := int64(0); i < size; i++ {
		reservation, err := cloud.client.RunInstances(&ec2.RunInstancesInput{
			ImageId:      cfg.ami,
			InstanceType: cfg.instanceType,
			MinCount:     aws.Int64(1),
			MaxCount:     aws.Int64(1),
			NetworkInterfaces: []*ec2.InstanceNetworkInterfaceSpecification{
				{
					AssociatePublicIpAddress: aws.Bool(true),
					DeviceIndex:              aws.Int64(0),
					Groups:                   []*string{cfg.securityGroupID},
					SubnetId:                 cfg.subnetID,
				},
			},
			BlockDeviceMappings: []*ec2.BlockDeviceMapping{
				{
					DeviceName: aws.String(fmt.Sprintf("/dev/sd%s", string(rune('f'+i)))),
					Ebs: &ec2.EbsBlockDevice{
						VolumeType: cfg.volumeType,
						VolumeSize: cfg.volumeSize,
					},
				},
			},
			KeyName: cfg.keyPair,
			TagSpecifications: []*ec2.TagSpecification{
				{
					ResourceType: aws.String(ec2.ResourceTypeInstance),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String(service.ClusterResourcesTagName),
							Value: aws.String(resourceId),
						},
					},
				},
			},
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				return nil, errors.New(aerr.Message())
			} else {
				return nil, err
			}
		}

		instanceIds = append(instanceIds, reservation.Instances[0].InstanceId)
	}
	if err := cloud.client.WaitUntilInstanceStatusOk(&ec2.DescribeInstanceStatusInput{
		InstanceIds: instanceIds,
	}); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("error occurred while waiting until instances running: InstanceIds:%v, Error:%w",
				instanceIds, err)
		}
	}
	instances, err := cloud.listInstances(instanceIds)
	if err != nil {
		return nil, errors.Wrap(err, "list instances")
	}
	return instances, nil
}

func (cloud *Cloud) listInstances(instanceIds []*string) ([]service.Instance, error) {
	instances := make([]service.Instance, 0, len(instanceIds))
	in := &ec2.DescribeInstancesInput{
		InstanceIds: instanceIds,
	}
	for i := 0; in.NextToken != nil || i == 0; i++ {
		describeInstances, err := cloud.client.DescribeInstances(in)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				return nil, errors.New(aerr.Message())
			}
			return nil, errors.Wrap(err, "describe instances failed")
		}
		for _, reservation := range describeInstances.Reservations {
			for _, instance := range reservation.Instances {
				instances = append(instances, service.Instance{
					PublicIpAddress:  aws.StringValue(instance.PublicIpAddress),
					PrivateIpAddress: aws.StringValue(instance.PrivateIpAddress),
				})
			}
		}
		in.NextToken = describeInstances.NextToken
	}
	return instances, nil
}

func (cloud *Cloud) keyPairPath(resourceId string) (string, error) {
	cfg := cloud.configs[resourceId]
	filePath, err := filepath.Abs(path.Join(aws.StringValue(cfg.pathToKeyPair), aws.StringValue(cfg.keyPair)+".pem"))
	if err != nil {
		return "", errors.Wrap(err, "failed to get absolute key pair path")
	}
	return filePath, nil
}
