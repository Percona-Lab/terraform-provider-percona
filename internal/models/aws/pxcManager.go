package aws

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"io/ioutil"
	"os"
	"terraform-percona/internal/utils/val"
	"time"
)

type XtraDBClusterManager struct {
	Config  *Config
	Session *session.Session
	Client  *ec2.EC2
}

type Config struct {
	Region    *string
	Profile   *string
	AccessKey *string
	SecretKey *string
	*InstanceSettings
}

type InstanceSettings struct {
	Ami                         *string
	InstanceType                *string
	MinCount                    *int64
	MaxCount                    *int64
	KeyPairName                 *string
	PathToClusterBoostrapScript *string
	PathToKeyPairStorage        *string
	ClusterSize                 *int64
}

const (
	Ami                          = "ami"
	InstanceType                 = "instance_type"
	MinCount                     = "min_count"
	MaxCount                     = "max_count"
	KeyPairName                  = "key_pair_name"
	PathToClusterBootstrapScript = "path_to_bootstrap_script"
	PathToKeyPairStorage         = "path_to_key_pair_storage"
	ClusterSize                  = "cluster_size"

	DurationBetweenInstanceRunning = 15 // seconds

	DefaultVpcCidrBlock    = "10.0.0.0/16"
	DefaultSubnetCidrBlock = "10.0.1.0/16"
	AllAddressesCidrBlock  = "0.0.0.0/0"

	SecurityGroupName        = "security-group"
	SecurityGroupDescription = "security-group"

	ErrorUserDataMsgFailedOpenFile   = "failed open file with user data"
	ErrorUserDataMsgFileNotExist     = "can't find user data file with proposed path"
	ErrorUserDataMsgPermissionDenied = "application doesn't have permission to open file with user data"
)

func (c *Config) Valid() bool {
	if c == nil {
		return false
	}

	return c.Region != nil
}

func NewXtraDBClusterManager(config *Config) (*XtraDBClusterManager, error) {
	if !config.Valid() {
		return &XtraDBClusterManager{}, errors.New("invalid config for Percona XtraDB Cluster manager")
	}
	sess, err := session.NewSession(&aws.Config{
		Region: config.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed create aws session: %w", err)
	}
	return &XtraDBClusterManager{
		Config:  config,
		Session: sess,
		Client:  ec2.New(sess),
	}, nil
}

func (manager *XtraDBClusterManager) CreateCluster() (interface{}, error) {
	//TODO full manager validation
	if manager.Client == nil {
		return nil, fmt.Errorf("nil EC2 client")
	}

	if err := manager.createKeyPair(); err != nil {
		return nil, err
	}

	vpc, err := manager.createVpc()
	if err != nil {
		return nil, err
	}
	fmt.Println(fmt.Sprintf("\t\t\tVPC\n%v\n\n", *vpc))

	internetGateway, err := manager.createInternetGateway(vpc)
	if err != nil {
		return nil, err
	}
	fmt.Println(fmt.Sprintf("\t\t\tInternet Gateway\n%v\n\n", *internetGateway))

	securityGroupId, err := manager.createSecurityGroup(vpc, aws.String(SecurityGroupName), aws.String(SecurityGroupDescription))
	if err != nil {
		return nil, err
	}
	fmt.Println(fmt.Sprintf("Security Group Id - %s\n\n", *securityGroupId))

	subnet, err := manager.createSubnet(vpc)
	if err != nil {
		return nil, err
	}
	fmt.Println(fmt.Sprintf("\t\t\tSubnet\n%v\n\n", *subnet))

	routeTable, err := manager.createRouteTable(vpc, internetGateway, subnet)
	if err != nil {
		return nil, err
	}
	fmt.Println(fmt.Sprintf("\t\t\tRouteTable\n%v\n\n", *routeTable))

	base64BootstrapData, err := manager.getBase64BoostrapData()
	if err != nil {
		return nil, err
	}

	for i := int64(0); i < *manager.Config.ClusterSize; i++ {
		reservation, err := manager.Client.RunInstances(&ec2.RunInstancesInput{
			ImageId:      manager.Config.Ami,
			InstanceType: manager.Config.InstanceType,
			MinCount:     manager.Config.MinCount,
			MaxCount:     manager.Config.MaxCount,
			NetworkInterfaces: []*ec2.InstanceNetworkInterfaceSpecification{
				{
					AssociatePublicIpAddress: aws.Bool(true),
					DeviceIndex:              aws.Int64(0),
					Groups:                   []*string{securityGroupId},
					SubnetId:                 subnet.SubnetId,
					PrivateIpAddress:         aws.String(fmt.Sprintf("10.0.1.%d", i+1)),
				},
			},
			KeyName:  manager.Config.KeyPairName,
			UserData: base64BootstrapData,
		})
		if err != nil {
			return nil, err
		}

		if err := manager.Client.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
			InstanceIds: []*string{reservation.Instances[0].InstanceId},
		}); err != nil {
			return nil, fmt.Errorf("error occurred while waiting until instance running: InstanceId:%s, Error:%w",
				*reservation.Instances[0].InstanceId, err)
		}
		time.Sleep(time.Second * DurationBetweenInstanceRunning)
		fmt.Println(fmt.Sprintf("Instance[%s] is running", *reservation.Instances[0].InstanceId))
	}
	return nil, nil
}

func (manager *XtraDBClusterManager) createKeyPair() error {
	//TODO add validation

	if val.Str(manager.Config.KeyPairName) == "" {
		return fmt.Errorf("cannot create key pair with empty name")
	}

	awsKeyPairStoragePath := fmt.Sprintf("%s%s.pem", *manager.Config.PathToKeyPairStorage, *manager.Config.KeyPairName)
	createKeyPairOutput, err := manager.Client.CreateKeyPair(&ec2.CreateKeyPairInput{
		KeyName: manager.Config.KeyPairName,
	})
	if err != nil {
		return fmt.Errorf("error occurred during key pair creating: %w", err)
	}

	if err := writeKey(awsKeyPairStoragePath, createKeyPairOutput.KeyMaterial); err != nil {
		return fmt.Errorf("failed write key pair to file: %w", err)
	}
	return nil
}

func writeKey(fileName string, fileData *string) error {
	err := os.WriteFile(fileName, []byte(*fileData), 0400)
	return err
}

func (manager *XtraDBClusterManager) createVpc() (*ec2.Vpc, error) {
	//TODO add validation

	createVpcOutput, err := manager.Client.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock: aws.String(DefaultVpcCidrBlock),
	})
	if err != nil {
		return nil, fmt.Errorf("error occurred during Vpc creating: %w", err)
	}

	if _, err = manager.Client.ModifyVpcAttribute(&ec2.ModifyVpcAttributeInput{
		EnableDnsHostnames: &ec2.AttributeBooleanValue{Value: aws.Bool(true)},
		VpcId:              createVpcOutput.Vpc.VpcId,
	}); err != nil {
		return nil, fmt.Errorf("failed modify Vpc attribute: VpcId:%s, Error:%w", *createVpcOutput.Vpc.VpcId, err)
	}

	return createVpcOutput.Vpc, nil
}

func (manager *XtraDBClusterManager) createInternetGateway(vpc *ec2.Vpc) (*ec2.InternetGateway, error) {
	//TODO add manager validation

	createInternetGatewayOutput, err := manager.Client.CreateInternetGateway(&ec2.CreateInternetGatewayInput{})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			//TODO add errors code cases
			switch aerr.Code() {
			default:
				return nil, fmt.Errorf("failed create internet gateway: %w", err)
			}
		}
		return nil, fmt.Errorf("failed create internet gateway: %w", err)
	}

	if _, err = manager.Client.AttachInternetGateway(&ec2.AttachInternetGatewayInput{
		InternetGatewayId: createInternetGatewayOutput.InternetGateway.InternetGatewayId,
		VpcId:             vpc.VpcId,
	}); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			//TODO add errors code cases
			switch aerr.Code() {
			default:
				return nil, fmt.Errorf("failed attach internet gateway to Vpc: VpcId:%s, Error:%w", *vpc.VpcId, err)
			}
		}
		return nil, fmt.Errorf("failed attach internet gateway to Vpc: VpcId:%s, Error:%w", *vpc.VpcId, err)
	}

	return createInternetGatewayOutput.InternetGateway, nil
}

func (manager *XtraDBClusterManager) createSecurityGroup(vpc *ec2.Vpc, groupName, groupDescription *string) (*string, error) {
	//TODO add manager validation

	createSecurityGroupResult, err := manager.Client.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
		GroupName:   groupName,
		Description: groupDescription,
		VpcId:       vpc.VpcId,
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case "InvalidVpcID.NotFound":
				return nil, fmt.Errorf("Unable to find VPC with ID %q. ", vpc.VpcId)
			case "InvalidGroup.Duplicate":
				return nil, fmt.Errorf("Security group %q already exists. ", *groupName)
			}
		}
		return nil, fmt.Errorf("Unable to create security group %q, %v ", *groupName, err)
	}

	if _, err = manager.Client.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: createSecurityGroupResult.GroupId,
		IpPermissions: []*ec2.IpPermission{
			(&ec2.IpPermission{}).
				SetIpProtocol("-1").
				SetFromPort(-1).
				SetToPort(-1).
				SetIpRanges([]*ec2.IpRange{
					{CidrIp: aws.String(AllAddressesCidrBlock)},
				}),
		},
	}); err != nil {
		return nil, fmt.Errorf("failed authorize security group ingress traffic: %w", err)
	}

	if _, err := manager.Client.AuthorizeSecurityGroupEgress(&ec2.AuthorizeSecurityGroupEgressInput{
		GroupId: createSecurityGroupResult.GroupId,
		IpPermissions: []*ec2.IpPermission{
			(&ec2.IpPermission{}).
				SetIpProtocol("-1").
				SetFromPort(-1).
				SetToPort(-1),
		},
	}); err != nil {
		return nil, fmt.Errorf("failed authorize security group egress traffic: %w", err)
	}

	return createSecurityGroupResult.GroupId, nil
}

func (manager *XtraDBClusterManager) createSubnet(vpc *ec2.Vpc) (*ec2.Subnet, error) {
	//TODO add manager validation

	createSubnetOutput, err := manager.Client.CreateSubnet(&ec2.CreateSubnetInput{
		CidrBlock: aws.String(DefaultSubnetCidrBlock),
		VpcId:     vpc.VpcId,
	})
	if err != nil {
		return nil, fmt.Errorf("failed create subnet: %w", err)
	}

	return createSubnetOutput.Subnet, nil
}

func (manager *XtraDBClusterManager) createRouteTable(vpc *ec2.Vpc, iGateway *ec2.InternetGateway, subnet *ec2.Subnet) (*ec2.RouteTable, error) {
	//TODO add manager validation

	createRouteTableOutput, err := manager.Client.CreateRouteTable(&ec2.CreateRouteTableInput{
		VpcId: vpc.VpcId,
	})
	if err != nil {
		return nil, fmt.Errorf("failed create route table: %w", err)
	}

	if _, err = manager.Client.CreateRoute(&ec2.CreateRouteInput{
		DestinationCidrBlock: aws.String(AllAddressesCidrBlock),
		GatewayId:            iGateway.InternetGatewayId,
		RouteTableId:         createRouteTableOutput.RouteTable.RouteTableId,
	}); err != nil {
		return nil, fmt.Errorf("failed create route: %w", err)
	}

	if _, err = manager.Client.AssociateRouteTable(&ec2.AssociateRouteTableInput{
		RouteTableId: createRouteTableOutput.RouteTable.RouteTableId,
		SubnetId:     subnet.SubnetId,
	}); err != nil {
		return nil, fmt.Errorf("failed associate route table: %w", err)
	}

	return createRouteTableOutput.RouteTable, nil
}

func (manager *XtraDBClusterManager) getBase64BoostrapData() (*string, error) {
	//TODO add manager validation

	file, err := os.Open(*manager.Config.PathToClusterBoostrapScript)
	if err != nil {
		switch err {
		case os.ErrNotExist:
			return nil, errors.New(ErrorUserDataMsgFileNotExist)
		case os.ErrPermission:
			return nil, errors.New(ErrorUserDataMsgPermissionDenied)
		default:
			return nil, errors.New(ErrorUserDataMsgFailedOpenFile)
		}
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	content, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return aws.String(base64.StdEncoding.EncodeToString(content)), nil
}
