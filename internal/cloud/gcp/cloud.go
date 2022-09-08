package gcp

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	"terraform-percona/internal/cloud"
	"terraform-percona/internal/service"
	"terraform-percona/internal/utils"
)

type Cloud struct {
	Project string
	Region  string
	Zone    string

	IgnoreErrorsOnDestroy bool

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
	vpcName        string
	subnetwork     string
}

func (c *Cloud) Configure(ctx context.Context, resourceId string, data *schema.ResourceData) error {
	if c.configs == nil {
		c.configs = make(map[string]*resourceConfig)
	}
	if _, ok := c.configs[resourceId]; !ok {
		c.configs[resourceId] = &resourceConfig{}
	}
	cfg := c.configs[resourceId]
	cfg.keyPair = data.Get(service.KeyPairName).(string)
	cfg.pathToKeyPair = data.Get(service.PathToKeyPairStorage).(string)
	cfg.configFilePath = data.Get(service.ConfigFilePath).(string)
	cfg.machineType = data.Get(service.InstanceType).(string)
	cfg.volumeType = data.Get(service.VolumeType).(string)
	if cfg.volumeType == "" {
		cfg.volumeType = "pd-balanced"
	}
	cfg.volumeSize = int64(data.Get(service.VolumeSize).(int))
	cfg.volumeIOPS = int64(data.Get(service.VolumeIOPS).(int))
	cfg.vpcName = data.Get(service.VPCName).(string)
	cfg.subnetwork = cfg.vpcName + "-sub"
	if cfg.vpcName == "" || cfg.vpcName == "default" {
		cfg.vpcName = "default"
		cfg.subnetwork = "default"
	}

	var err error
	c.client, err = compute.NewService(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create compute client")
	}
	return nil
}

func (c *Cloud) createVPCIfNotExists(ctx context.Context, cfg *resourceConfig) error {
	vpc, err := c.client.Networks.Get(c.Project, cfg.vpcName).Context(ctx).Do()
	if err != nil {
		gerr, ok := err.(*googleapi.Error)
		if (ok && gerr.Code != http.StatusNotFound) || !ok {
			return errors.Wrap(err, "failed to get vpc")
		}
	}
	if vpc != nil {
		return nil
	}
	vpc = &compute.Network{
		Name: cfg.vpcName,
		RoutingConfig: &compute.NetworkRoutingConfig{
			RoutingMode: "REGIONAL",
		},
		AutoCreateSubnetworks:                 false,
		ForceSendFields:                       []string{"AutoCreateSubnetworks"},
		NetworkFirewallPolicyEnforcementOrder: "AFTER_CLASSIC_FIREWALL",
		Mtu:                                   1460,
	}
	if err = doUntilStatus(ctx, c.client.Networks.Insert(c.Project, vpc).Context(ctx), "DONE"); err != nil {
		return errors.Wrap(err, "failed to create vpc")
	}
	return nil
}

func (c *Cloud) CreateInstances(ctx context.Context, resourceId string, size int64) ([]cloud.Instance, error) {
	cfg := c.configs[resourceId]
	publicKey := user + ":" + cfg.publicKey
	diskType := path.Join("projects", c.Project, "zones", c.Zone, "diskTypes", cfg.volumeType)
	subnetwork := path.Join("projects", c.Project, "regions", c.Region, "subnetworks", cfg.subnetwork)
	machineTypePath := path.Join("projects", c.Project, "zones", c.Zone, "machineTypes", cfg.machineType)

	g, gCtx := errgroup.WithContext(ctx)
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
		g.Go(func() error {
			if err := doUntilStatus(gCtx, c.client.Instances.Insert(c.Project, c.Zone, instance).Context(gCtx), "RUNNING"); err != nil {
				return errors.Wrapf(err, "failed to create instance %s", instance.Name)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, errors.Wrap(err, "failed to create instances")
	}

	if err := c.waitUntilAllInstancesAreReady(ctx, resourceId); err != nil {
		return nil, errors.Wrap(err, "failed to wait instances")
	}
	var instances []cloud.Instance
	list, err := c.listInstances(ctx, resourceId)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list instances")
	}
	for _, v := range list {
		instances = append(instances, cloud.Instance{
			PrivateIpAddress: v.NetworkInterfaces[0].NetworkIP,
			PublicIpAddress:  v.NetworkInterfaces[0].AccessConfigs[0].NatIP,
		})
	}
	return instances, nil
}

func (c *Cloud) waitUntilAllInstancesAreReady(ctx context.Context, resourceId string) error {
	sshConfig, err := c.sshConfig(resourceId)
	if err != nil {
		return errors.Wrap(err, "ssh config")
	}
	for {
		shouldExit := true
		list, err := c.listInstances(ctx, resourceId)
		if err != nil {
			return errors.Wrap(err, "failed to list instances")
		}
		for _, v := range list {
			host := v.NetworkInterfaces[0].AccessConfigs[0].NatIP
			if host == "" {
				shouldExit = false
				break
			}
			if err = utils.SSHPing(ctx, host, sshConfig); err != nil {
				shouldExit = false
				break
			}
		}
		if shouldExit {
			return nil
		}
		time.Sleep(time.Second)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (c *Cloud) listInstances(ctx context.Context, resourceId string) ([]compute.Instance, error) {
	var instances []compute.Instance
	nextPageToken := ""
	var callOptions []googleapi.CallOption
	for {
		list, err := c.client.Instances.List(c.Project, c.Zone).Context(ctx).Filter("labels." + service.ClusterResourcesTagName + ":" + strings.ToLower(resourceId)).Do(callOptions...)
		if err != nil {
			return nil, err
		}
		for _, instance := range list.Items {
			instances = append(instances, *instance)
		}
		if list.NextPageToken == "" || len(list.Items) == 0 {
			break
		}
		nextPageToken = list.NextPageToken
		callOptions = []googleapi.CallOption{googleapi.QueryParameter("pageToken", nextPageToken)}
	}
	return instances, nil
}

func (c *Cloud) CreateInfrastructure(ctx context.Context, resourceId string) error {
	sshKeyPath, err := c.keyPairPath(resourceId)
	if err != nil {
		return errors.Wrap(err, "key pair path")
	}
	cfg := c.configs[resourceId]
	cfg.publicKey, err = utils.GetSSHPublicKey(sshKeyPath)
	if err != nil {
		return errors.Wrap(err, "failed to create SSH key")
	}

	if err = c.createVPCIfNotExists(ctx, cfg); err != nil {
		return errors.Wrap(err, "failed to create vpc")
	}
	if err = c.createSubnetworkIfNotExists(ctx, cfg.subnetwork, cfg.vpcName); err != nil {
		return errors.Wrap(err, "failed to create subnetwork")
	}
	if err = c.createFirewallIfNotExists(ctx, cfg.vpcName+"-allow-all", cfg.vpcName); err != nil {
		return errors.Wrap(err, "failed to create firewall")
	}
	return nil
}

func (c *Cloud) createFirewallIfNotExists(ctx context.Context, firewallName, vpcName string) error {
	firewall, err := c.client.Firewalls.Get(c.Project, firewallName).Context(ctx).Do()
	if err != nil {
		gerr, ok := err.(*googleapi.Error)
		if (ok && gerr.Code != http.StatusNotFound) || !ok {
			return errors.Wrap(err, "failed to get subnetwork")
		}
	}
	if firewall != nil {
		return nil
	}
	firewall = &compute.Firewall{
		Name:      firewallName,
		Direction: "INGRESS",
		Network:   path.Join("projects", c.Project, "global", "networks", vpcName),
		Priority:  65534,
		SourceRanges: []string{
			service.AllAddressesCidrBlock,
		},
		Allowed: []*compute.FirewallAllowed{
			{
				IPProtocol: "all",
			},
		},
	}
	if err = doUntilStatus(ctx, c.client.Firewalls.Insert(c.Project, firewall).Context(ctx), "DONE"); err != nil {
		return errors.Wrap(err, "failed to doUntilStatus firewall")
	}
	return nil
}

func (c *Cloud) createSubnetworkIfNotExists(ctx context.Context, subnetworkName, vpcName string) error {
	subnetwork, err := c.client.Subnetworks.Get(c.Project, c.Region, subnetworkName).Context(ctx).Do()
	if err != nil {
		gerr, ok := err.(*googleapi.Error)
		if (ok && gerr.Code != http.StatusNotFound) || !ok {
			return errors.Wrap(err, "failed to get subnetwork")
		}
	}
	if subnetwork != nil {
		return nil
	}
	subnetwork = &compute.Subnetwork{
		Name:                  subnetworkName,
		IpCidrRange:           service.DefaultVpcCidrBlock,
		Network:               path.Join("projects", c.Project, "global", "networks", vpcName),
		PrivateIpGoogleAccess: true,
		Region:                path.Join("projects", c.Project, "regions", c.Region),
		StackType:             "IPV4_ONLY",
	}
	if err = doUntilStatus(ctx, c.client.Subnetworks.Insert(c.Project, c.Region, subnetwork).Context(ctx), "DONE"); err != nil {
		return errors.Wrap(err, "failed to create subnetwork")
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
	sshConfig, err := c.sshConfig(resourceId)
	if err != nil {
		return "", errors.Wrap(err, "ssh config")
	}
	return utils.RunCommand(ctx, cmd, instance.PublicIpAddress, sshConfig)
}

func (c *Cloud) SendFile(ctx context.Context, resourceId, filePath, remotePath string, instance cloud.Instance) error {
	sshConfig, err := c.sshConfig(resourceId)
	if err != nil {
		return errors.Wrap(err, "ssh config")
	}
	return utils.SendFile(ctx, filePath, remotePath, instance.PublicIpAddress, sshConfig)
}

func (c *Cloud) DeleteInfrastructure(ctx context.Context, resourceId string) error {
	cfg := c.configs[resourceId]
	list, err := c.listInstances(ctx, resourceId)
	if err != nil {
		return errors.Wrap(err, "failed to list instances")
	}
	g, gCtx := errgroup.WithContext(ctx)
	for _, v := range list {
		instanceName := v.Name
		g.Go(func() error {
			if err = doUntilStatus(gCtx, c.client.Instances.Delete(c.Project, c.Zone, instanceName).Context(gCtx), "DONE"); err != nil {
				if !c.IgnoreErrorsOnDestroy {
					return errors.Wrapf(err, "delete %s instance", instanceName)
				} else {
					tflog.Error(gCtx, "failed to delete instance", map[string]interface{}{
						"instance": instanceName, "error": err.Error(),
					})
				}
			}
			return nil
		})
	}
	if err = g.Wait(); err != nil {
		return errors.Wrap(err, "failed to delete instances")
	}

	if cfg.vpcName != "default" {
		if err = doUntilStatus(ctx, c.client.Firewalls.Delete(c.Project, cfg.vpcName+"-allow-all").Context(ctx), "DONE"); err != nil {
			if err.Error() != errNotFound {
				if !c.IgnoreErrorsOnDestroy {
					return errors.Wrap(err, "failed to delete firewall")
				}
				tflog.Error(ctx, "failed to delete firewall", map[string]interface{}{
					"firewall": cfg.vpcName + "-allow-all", "error": err.Error(),
				})
			}
		}

		if err = doUntilStatus(ctx, c.client.Subnetworks.Delete(c.Project, c.Region, cfg.subnetwork).Context(ctx), "DONE"); err != nil {
			if err.Error() != errNotFound {
				if !c.IgnoreErrorsOnDestroy {
					return errors.Wrap(err, "failed to delete subnetwork")
				}
				tflog.Error(ctx, "failed to delete subnetwork", map[string]interface{}{
					"subnetwork": cfg.subnetwork, "error": err.Error(),
				})
			}
		}

		if err = doUntilStatus(ctx, c.client.Networks.Delete(c.Project, cfg.vpcName).Context(ctx), "DONE"); err != nil {
			if err.Error() != errNotFound {
				if !c.IgnoreErrorsOnDestroy {
					return errors.Wrap(err, "failed to delete vpc")
				}
				tflog.Error(ctx, "failed to delete vpc", map[string]interface{}{
					"vpc": cfg.vpcName, "error": err.Error(),
				})
			}
		}
	}

	return nil
}

func (c *Cloud) sshConfig(resourceId string) (*ssh.ClientConfig, error) {
	sshKeyPath, err := c.keyPairPath(resourceId)
	if err != nil {
		return nil, errors.Wrap(err, "get key pair path")
	}
	sshConfig, err := utils.SSHConfig(user, sshKeyPath)
	if err != nil {
		return nil, errors.Wrap(err, "get ssh config")
	}
	return sshConfig, nil
}
