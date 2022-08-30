package gcp

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	"google.golang.org/api/compute/v1"
	"path"
	"path/filepath"
	"strings"
	"terraform-percona/internal/cloud"
	"terraform-percona/internal/service"
	"terraform-percona/internal/utils"
	"time"
)

type Cloud struct {
	Project string
	Region  string
	Zone    string

	client *compute.Service

	configs map[string]*resourceConfig
}

type resourceConfig struct {
	keyPair        string
	pathToKeyPair  string
	configFilePath string
	machineType    string
	publicKey      string
	volumeType     string
	volumeSize     int64
	volumeIOPS     int64
}

func (c *Cloud) Configure(ctx context.Context, resourceId string, data *schema.ResourceData) error {
	if c.configs == nil {
		c.configs = make(map[string]*resourceConfig)
	}
	if _, ok := c.configs[resourceId]; !ok {
		c.configs[resourceId] = &resourceConfig{}
	}
	cfg := c.configs[resourceId]
	if v, ok := data.Get(service.KeyPairName).(string); ok {
		cfg.keyPair = v
	}

	if v, ok := data.Get(service.PathToKeyPairStorage).(string); ok {
		cfg.pathToKeyPair = v
	}

	if v, ok := data.Get(service.ConfigFilePath).(string); ok {
		cfg.configFilePath = v
	}

	if v, ok := data.Get(service.InstanceType).(string); ok {
		cfg.machineType = v
	}

	if v, ok := data.Get(service.VolumeType).(string); ok {
		cfg.volumeType = v
	}
	if cfg.volumeType == "" {
		cfg.volumeType = "pd-balanced"
	}

	if v, ok := data.Get(service.VolumeSize).(int); ok {
		cfg.volumeSize = int64(v)
	}

	if v, ok := data.Get(service.VolumeIOPS).(int); ok {
		cfg.volumeIOPS = int64(v)
	}

	var err error
	c.client, err = compute.NewService(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create compute client")
	}
	return nil
}

const sourceImage = "projects/ubuntu-os-cloud/global/images/ubuntu-minimal-2004-focal-v20220713"

func (c *Cloud) CreateInstances(ctx context.Context, resourceId string, size int64) ([]cloud.Instance, error) {
	cfg := c.configs[resourceId]
	publicKey := "ubuntu:" + cfg.publicKey
	diskType := path.Join("projects", c.Project, "zones", c.Zone, "diskTypes", cfg.volumeType)
	subnetwork := path.Join("projects", c.Project, "regions", c.Region, "subnetworks", "default")
	machineTypePath := path.Join("projects", c.Project, "zones", c.Zone, "machineTypes", cfg.machineType)

	for i := int64(0); i < size; i++ {
		name := strings.ToLower(fmt.Sprintf("instance-%s-%d", resourceId, i))
		instance := &compute.Instance{
			Name:        name,
			MachineType: machineTypePath,
			Disks: []*compute.AttachedDisk{
				{
					AutoDelete: true,
					Boot:       true,
					Type:       "PERSISTENT",
					InitializeParams: &compute.AttachedDiskInitializeParams{
						DiskName:        name,
						DiskType:        diskType,
						DiskSizeGb:      cfg.volumeSize,
						ProvisionedIops: cfg.volumeIOPS,
						SourceImage:     sourceImage,
					},
					DiskEncryptionKey: new(compute.CustomerEncryptionKey),
				},
			},
			Metadata: &compute.Metadata{
				Items: []*compute.MetadataItems{
					{
						Key:   "ssh-keys",
						Value: &publicKey,
					},
				},
			},
			NetworkInterfaces: []*compute.NetworkInterface{
				{
					AccessConfigs: []*compute.AccessConfig{
						{
							Name:        "External NAT",
							NetworkTier: "PREMIUM",
						},
					},
					StackType:  "IPV4_ONLY",
					Subnetwork: subnetwork,
				},
			},
			Labels: map[string]string{
				service.ClusterResourcesTagName: strings.ToLower(resourceId),
			},
			Zone: path.Join("projects", c.Project, "zones", c.Zone),
		}
		_, err := c.client.Instances.Insert(c.Project, c.Zone, instance).Context(ctx).Do()
		if err != nil {
			return nil, errors.Wrap(err, "failed to insert instance")
		}
	}

	return c.waitUntilAllInstancesReady(ctx, resourceId, int(size))
}

func (c *Cloud) waitUntilAllInstancesReady(ctx context.Context, resourceId string, size int) ([]cloud.Instance, error) {
	for {
		time.Sleep(time.Second)
		list, err := c.client.Instances.List(c.Project, c.Zone).Context(ctx).Filter("labels." + service.ClusterResourcesTagName + ":" + strings.ToLower(resourceId)).Do()
		if err != nil {
			return nil, errors.Wrap(err, "failed to list instances")
		}
		shouldExit := true
		for _, v := range list.Items {
			if v.Status != "RUNNING" {
				shouldExit = false
			}
		}
		if len(list.Items) != size {
			shouldExit = false
		}
		if shouldExit {
			instances := make([]cloud.Instance, len(list.Items))
			for i, v := range list.Items {
				instances[i] = cloud.Instance{
					PrivateIpAddress: v.NetworkInterfaces[0].NetworkIP,
					PublicIpAddress:  v.NetworkInterfaces[0].AccessConfigs[0].NatIP,
				}
			}
			select {
			case <-ctx.Done():
				return nil, nil
			case <-time.After(time.Second * 60):
			}
			return instances, nil
		}
	}
}

func (c *Cloud) CreateInfrastructure(_ context.Context, resourceId string) error {
	sshKeyPath, err := c.keyPairPath(resourceId)
	if err != nil {
		return errors.Wrap(err, "key pair path")
	}
	cfg := c.configs[resourceId]
	cfg.publicKey, err = utils.GetSSHPublicKey(sshKeyPath)
	if err != nil {
		return errors.Wrap(err, "failed to create SSH key")
	}
	return nil
}

func (c *Cloud) keyPairPath(resourceId string) (string, error) {
	cfg := c.configs[resourceId]
	filePath, err := filepath.Abs(path.Join(cfg.pathToKeyPair, cfg.keyPair+".pem"))
	if err != nil {
		return "", errors.Wrap(err, "failed to get absolute key pair path")
	}
	return filePath, nil
}

func (c *Cloud) RunCommand(ctx context.Context, resourceId string, instance cloud.Instance, cmd string) (string, error) {
	sshKeyPath, err := c.keyPairPath(resourceId)
	if err != nil {
		return "", err
	}
	sshConfig, err := utils.SSHConfig("ubuntu", sshKeyPath)
	if err != nil {
		return "", errors.Wrap(err, "ssh config")
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
func (c *Cloud) DeleteInfrastructure(ctx context.Context, resourceId string) error {
	list, err := c.client.Instances.List(c.Project, c.Zone).Context(ctx).Filter("labels." + service.ClusterResourcesTagName + ":" + resourceId).Do()
	if err != nil {
		return errors.Wrap(err, "failed to list instances")
	}
	for _, v := range list.Items {
		if _, err := c.client.Instances.Delete(c.Project, c.Zone, v.Name).Context(ctx).Do(); err != nil {
			return errors.Wrapf(err, "delete %s instance", v.Name)
		}
	}
	return nil
}
