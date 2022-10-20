package aws

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"

	"terraform-percona/internal/cloud"
	"terraform-percona/internal/resource"
	"terraform-percona/internal/utils"
)

type Cloud struct {
	Region  *string
	Profile *string

	IgnoreErrorsOnDestroy bool

	client  *ec2.EC2
	session *session.Session

	configs map[string]*resourceConfig
	configsMu sync.Mutex
}

type resourceConfig struct {
	keyPair          *string
	pathToKeyPair    *string
	securityGroupID  *string
	subnetID         *string
	ami              *string
	instanceType     *string
	volumeSize       *int64
	volumeType       *string
	volumeIOPS       *int64
	volumeThroughput *int64
	vpcName          *string
	vpcId            *string
}

func (c *Cloud) config(resourceId string) *resourceConfig {
	c.configsMu.Lock()
	if c.configs == nil {
		c.configs = make(map[string]*resourceConfig)
	}
	res, ok := c.configs[resourceId]
	if !ok {
		res = new(resourceConfig)
		c.configs[resourceId] = res
	}
	c.configsMu.Unlock()
	return res
}

func (c *Cloud) RunCommand(ctx context.Context, resourceId string, instance cloud.Instance, cmd string) (string, error) {
	sshKeyPath, err := c.keyPairPath(resourceId)
	if err != nil {
		return "", errors.Wrap(err, "get key pair path")
	}
	sshConfig, err := utils.SSHConfig("ubuntu", sshKeyPath)
	if err != nil {
		return "", errors.Wrap(err, "get ssh config")
	}
	return utils.RunCommand(ctx, cmd, instance.PublicIpAddress, sshConfig)
}

func (c *Cloud) SendFile(ctx context.Context, resourceId, filePath, remotePath string, instance cloud.Instance) error {
	sshKeyPath, err := c.keyPairPath(resourceId)
	if err != nil {
		return errors.Wrap(err, "get key pair path")
	}
	sshConfig, err := utils.SSHConfig("ubuntu", sshKeyPath)
	if err != nil {
		return errors.Wrap(err, "get ssh config")
	}
	return utils.SendFile(ctx, filePath, remotePath, instance.PublicIpAddress, sshConfig)
}

func (c *Cloud) CreateInstances(ctx context.Context, resourceId string, size int64) ([]cloud.Instance, error) {
	instanceIds := make([]*string, 0, size)
	cfg := c.config(resourceId)
	reservation, err := c.client.RunInstances(&ec2.RunInstancesInput{
		ImageId:      cfg.ami,
		InstanceType: cfg.instanceType,
		MinCount:     aws.Int64(size),
		MaxCount:     aws.Int64(size),
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
				DeviceName: aws.String("/dev/sda1"),
				Ebs: &ec2.EbsBlockDevice{
					VolumeType: cfg.volumeType,
					VolumeSize: cfg.volumeSize,
					Iops:       cfg.volumeIOPS,
					Throughput: cfg.volumeThroughput,
				},
			},
		},
		KeyName: cfg.keyPair,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeInstance),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(resource.TagName),
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

	for _, instance := range reservation.Instances {
		instanceIds = append(instanceIds, instance.InstanceId)
	}
	if err := c.client.WaitUntilInstanceStatusOkWithContext(ctx, &ec2.DescribeInstanceStatusInput{
		InstanceIds: instanceIds,
	}); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("error occurred while waiting until instances running: InstanceIds:%v, Error:%w",
				instanceIds, err)
		}
	}
	instances, err := c.listInstances(ctx, instanceIds)
	if err != nil {
		return nil, errors.Wrap(err, "list instances")
	}
	return instances, nil
}

func (c *Cloud) listInstances(ctx context.Context, instanceIds []*string) ([]cloud.Instance, error) {
	instances := make([]cloud.Instance, 0, len(instanceIds))
	in := &ec2.DescribeInstancesInput{
		InstanceIds: instanceIds,
	}
	for i := 0; in.NextToken != nil || i == 0; i++ {
		describeInstances, err := c.client.DescribeInstancesWithContext(ctx, in)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				return nil, errors.New(aerr.Message())
			}
			return nil, errors.Wrap(err, "describe instances failed")
		}
		for _, reservation := range describeInstances.Reservations {
			for _, instance := range reservation.Instances {
				instances = append(instances, cloud.Instance{
					PublicIpAddress:  aws.StringValue(instance.PublicIpAddress),
					PrivateIpAddress: aws.StringValue(instance.PrivateIpAddress),
				})
			}
		}
		in.NextToken = describeInstances.NextToken
	}
	return instances, nil
}

func (c *Cloud) keyPairPath(resourceId string) (string, error) {
	cfg := c.config(resourceId)
	filePath, err := filepath.Abs(path.Join(aws.StringValue(cfg.pathToKeyPair), aws.StringValue(cfg.keyPair)+".pem"))
	if err != nil {
		return "", errors.Wrap(err, "failed to get absolute key pair path")
	}
	return filePath, nil
}
