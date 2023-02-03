package gcp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"github.com/googleapis/gax-go/v2"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"

	"terraform-percona/internal/cloud"
	"terraform-percona/internal/resource"
	"terraform-percona/internal/utils"
)

type Cloud struct {
	Project string
	Region  string
	Zone    string

	Meta cloud.Metadata

	client struct {
		Instances   *compute.InstancesClient
		Networks    *compute.NetworksClient
		Subnetworks *compute.SubnetworksClient
		Firewalls   *compute.FirewallsClient
	}

	configs   map[string]*resourceConfig
	configsMu sync.Mutex
	infraMu   sync.Mutex
}

type resourceConfig struct {
	keyPair       string
	pathToKeyPair string
	machineType   string
	publicKey     string
	volumeType    string
	volumeSize    int64
	volumeIOPS    *int64
	vpcName       string
	subnetwork    string
}

func (c *Cloud) config(resourceID string) *resourceConfig {
	c.configsMu.Lock()
	if c.configs == nil {
		c.configs = make(map[string]*resourceConfig)
	}
	res, ok := c.configs[resourceID]
	if !ok {
		res = new(resourceConfig)
		c.configs[resourceID] = res
	}
	c.configsMu.Unlock()
	return res
}

func (c *Cloud) Metadata() cloud.Metadata {
	return c.Meta
}

func (c *Cloud) Configure(ctx context.Context, resourceID string, data *schema.ResourceData) error {
	cfg := c.config(resourceID)
	if data != nil {
		cfg.keyPair = data.Get(resource.SchemaKeyKeyPairName).(string)
		cfg.pathToKeyPair = data.Get(resource.SchemaKeyPathToKeyPairStorage).(string)
		cfg.machineType = data.Get(resource.SchemaKeyInstanceType).(string)
		cfg.volumeType = data.Get(resource.SchemaKeyVolumeType).(string)
		if cfg.volumeType == "" {
			cfg.volumeType = "pd-balanced"
		}
		cfg.volumeSize = int64(data.Get(resource.SchemaKeyVolumeSize).(int))
		if volumeIOPS := int64(data.Get(resource.SchemaKeyVolumeIOPS).(int)); volumeIOPS != 0 {
			cfg.volumeIOPS = &volumeIOPS
		}
		cfg.vpcName = data.Get(resource.SchemaKeyVPCName).(string)
	}
	cfg.subnetwork = cfg.vpcName + "-sub"
	if cfg.vpcName == "" || cfg.vpcName == "default" {
		cfg.vpcName = "default"
		cfg.subnetwork = "default"
	}

	var err error
	c.client.Instances, err = compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create instances client")
	}
	runtime.SetFinalizer(c.client.Instances, func(obj *compute.InstancesClient) {
		if err := obj.Close(); err != nil {
			tflog.Error(ctx, "failed to close instances client")
		}
	})
	c.client.Networks, err = compute.NewNetworksRESTClient(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create networks client")
	}
	runtime.SetFinalizer(c.client.Networks, func(obj *compute.NetworksClient) {
		if err := obj.Close(); err != nil {
			tflog.Error(ctx, "failed to close networks client")
		}
	})
	c.client.Subnetworks, err = compute.NewSubnetworksRESTClient(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create subnetworks client")
	}
	runtime.SetFinalizer(c.client.Subnetworks, func(obj *compute.SubnetworksClient) {
		if err := obj.Close(); err != nil {
			tflog.Error(ctx, "failed to close subnetworks client")
		}
	})
	c.client.Firewalls, err = compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create firewalls client")
	}
	runtime.SetFinalizer(c.client.Firewalls, func(obj *compute.FirewallsClient) {
		if err := obj.Close(); err != nil {
			tflog.Error(ctx, "failed to close firewalls client")
		}
	})
	return nil
}

func (c *Cloud) createVPCIfNotExists(ctx context.Context, cfg *resourceConfig) error {
	vpc, err := c.client.Networks.Get(ctx, &computepb.GetNetworkRequest{
		Project: c.Project,
		Network: cfg.vpcName,
	})
	if err != nil {
		var gerr *googleapi.Error
		if ok := errors.As(err, &gerr); (ok && gerr.Code != http.StatusNotFound) || !ok {
			return errors.Wrapf(err, "failed to get vpc %b", ok)
		}
	}
	if vpc != nil {
		return nil
	}
	vpc = &computepb.Network{
		Name: utils.Ref(cfg.vpcName),
		RoutingConfig: &computepb.NetworkRoutingConfig{
			RoutingMode: utils.Ref("REGIONAL"),
		},
		AutoCreateSubnetworks:                 utils.Ref(false),
		NetworkFirewallPolicyEnforcementOrder: utils.Ref("AFTER_CLASSIC_FIREWALL"),
		Mtu:                                   utils.Ref(int32(1460)),
	}
	op, err := c.client.Networks.Insert(ctx, &computepb.InsertNetworkRequest{
		NetworkResource: vpc,
		Project:         c.Project,
	})
	if err != nil {
		return errors.Wrap(err, "failed to insert vpc")
	}
	if err = op.Wait(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for vpc")
	}
	return nil
}

func (c *Cloud) sourceImageURI(ctx context.Context) (string, error) {
	cli, err := compute.NewImagesRESTClient(ctx)
	if err != nil {
		return "", errors.Wrap(err, "new image rest client")
	}
	defer cli.Close()
	filter := `name eq 'ubuntu-minimal-2004-focal-v.*'`

	req := &computepb.ListImagesRequest{
		Filter:  &filter,
		Project: "ubuntu-os-cloud",
	}
	var images []*computepb.Image
	it := cli.List(ctx, req)
	for {
		image, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", errors.Wrap(err, "compute image list")
		}
		if image.Status != nil && *image.Status == "READY" {
			images = append(images, image)
		}
	}
	if len(images) == 0 {
		return "", errors.New("image not found")
	}
	sort.Slice(images, func(i, j int) bool {
		return images[i].GetName() > images[j].GetName()
	})
	return images[0].GetSelfLink(), nil
}

func (c *Cloud) CreateInstances(ctx context.Context, resourceID string, size int64, labels map[string]string) ([]cloud.Instance, error) {
	cfg := c.config(resourceID)
	publicKey := user + ":" + cfg.publicKey
	subnetwork := path.Join("projects", c.Project, "regions", c.Region, "subnetworks", cfg.subnetwork)
	sourceImage, err := c.sourceImageURI(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get latest Ubuntu 20.04 image")
	}

	labels = utils.MapMerge(labels, map[string]string{
		resource.LabelKeyResourceID: strings.ToLower(resourceID),
	})

	op, err := c.client.Instances.BulkInsert(ctx, &computepb.BulkInsertInstanceRequest{
		BulkInsertInstanceResourceResource: &computepb.BulkInsertInstanceResource{
			Count: utils.Ref(size),
			InstanceProperties: &computepb.InstanceProperties{
				Disks: []*computepb.AttachedDisk{
					{
						AutoDelete: utils.Ref(true),
						Boot:       utils.Ref(true),
						Type:       utils.Ref("PERSISTENT"),
						InitializeParams: &computepb.AttachedDiskInitializeParams{
							DiskType:        utils.Ref(cfg.volumeType),
							DiskSizeGb:      utils.Ref(cfg.volumeSize),
							ProvisionedIops: cfg.volumeIOPS,
							SourceImage:     utils.Ref(sourceImage),
						},
						DiskEncryptionKey: new(computepb.CustomerEncryptionKey),
					},
				},
				Labels:      labels,
				MachineType: utils.Ref(cfg.machineType),
				Metadata: &computepb.Metadata{
					Items: []*computepb.Items{
						{
							Key:   utils.Ref("ssh-keys"),
							Value: &publicKey,
						},
					},
				},
				NetworkInterfaces: []*computepb.NetworkInterface{
					{
						AccessConfigs: []*computepb.AccessConfig{
							{
								Name:        utils.Ref("External NAT"),
								NetworkTier: utils.Ref("PREMIUM"),
								Type:        utils.Ref("ONE_TO_ONE_NAT"),
							},
						},
						StackType:  utils.Ref("IPV4_ONLY"),
						Subnetwork: utils.Ref(subnetwork),
					},
				},
			},
			MinCount:    utils.Ref(size),
			NamePattern: utils.Ref(instanceNamePattern(resourceID)),
		},
		Project: c.Project,
		Zone:    c.Zone,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to insert instances")
	}
	if err := op.Wait(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait instances")
	}

	if err := c.waitUntilAllInstancesAreReady(ctx, resourceID, labels); err != nil {
		return nil, errors.Wrap(err, "failed to wait instances")
	}
	instances, err := c.ListInstances(ctx, resourceID, labels)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list instances")
	}
	return instances, nil
}

func instanceNamePattern(resourceID string) string {
	return strings.ToLower(fmt.Sprintf("instance-%s-#", resourceID))
}

func (c *Cloud) waitUntilAllInstancesAreReady(ctx context.Context, resourceID string, labels map[string]string) error {
	sshConfig, err := c.sshConfig(resourceID)
	if err != nil {
		return errors.Wrap(err, "ssh config")
	}
	for {
		isReady := true
		instances, err := c.ListInstances(ctx, resourceID, labels)
		if err != nil {
			return errors.Wrap(err, "failed to list instances")
		}
		if len(instances) == 0 {
			if err = gax.Sleep(ctx, time.Second); err != nil {
				return err
			}
			continue
		}
		for _, instance := range instances {
			host := instance.PublicIpAddress
			if host == "" {
				isReady = false
				break
			}
			if err = utils.SSHPing(ctx, host, sshConfig); err != nil {
				isReady = false
				break
			}
		}
		if isReady {
			return nil
		}
		if err = gax.Sleep(ctx, time.Second); err != nil {
			return err
		}
	}
}

func (c *Cloud) listInstances(ctx context.Context, resourceID string, labels map[string]string) ([]*computepb.Instance, error) {
	var instances []*computepb.Instance

	labels = utils.MapMerge(labels, map[string]string{
		resource.LabelKeyResourceID: strings.ToLower(resourceID),
	})

	var fb strings.Builder
	i := 0
	for k, v := range labels {
		if i > 0 {
			fb.WriteString(" AND ")
		}
		fb.WriteString("labels.")
		fb.WriteString(k)
		fb.WriteString(":")
		fb.WriteString(v)
		i++
	}
	var filter *string
	if fb.Len() > 0 {
		filter = utils.Ref(fb.String())
	}
	it := c.client.Instances.List(ctx, &computepb.ListInstancesRequest{
		Filter:  filter,
		Project: c.Project,
		Zone:    c.Zone,
	})
	for {
		instance, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "next page")
		}
		instances = append(instances, instance)
	}
	return instances, nil
}

func (c *Cloud) ListInstances(ctx context.Context, resourceID string, labels map[string]string) ([]cloud.Instance, error) {
	var instances []cloud.Instance

	pbInstances, err := c.listInstances(ctx, resourceID, labels)
	if err != nil {
		return nil, err
	}
	for _, instance := range pbInstances {
		instances = append(instances, cloud.Instance{
			PrivateIpAddress: *instance.NetworkInterfaces[0].NetworkIP,
			PublicIpAddress:  *instance.NetworkInterfaces[0].AccessConfigs[0].NatIP,
		})
	}
	return instances, nil
}

func (c *Cloud) CreateInfrastructure(ctx context.Context, resourceID string) error {
	c.infraMu.Lock()

	sshKeyPath, err := c.keyPairPath(resourceID)
	if err != nil {
		return errors.Wrap(err, "key pair path")
	}
	cfg := c.config(resourceID)
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

	c.infraMu.Unlock()
	return nil
}

func (c *Cloud) createFirewallIfNotExists(ctx context.Context, firewallName, vpcName string) error {
	client, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create instances client")
	}
	defer client.Close()
	firewall, err := client.Get(ctx, &computepb.GetFirewallRequest{
		Firewall: firewallName,
		Project:  c.Project,
	})
	if err != nil {
		var gerr *googleapi.Error
		if ok := errors.As(err, &gerr); (ok && gerr.Code != http.StatusNotFound) || !ok {
			return errors.Wrap(err, "failed to get subnetwork")
		}
	}
	if firewall != nil {
		return nil
	}
	firewall = &computepb.Firewall{
		Name:      utils.Ref(firewallName),
		Direction: utils.Ref("INGRESS"),
		Network:   utils.Ref(path.Join("projects", c.Project, "global", "networks", vpcName)),
		Priority:  utils.Ref(int32(65534)),
		SourceRanges: []string{
			cloud.AllAddressesCidrBlock,
		},
		Allowed: []*computepb.Allowed{
			{
				IPProtocol: utils.Ref("all"),
			},
		},
	}
	op, err := client.Insert(ctx, &computepb.InsertFirewallRequest{
		FirewallResource: firewall,
		Project:          c.Project,
	})
	if err != nil {
		return errors.Wrap(err, "failed to insert firewall")
	}
	if err = op.Wait(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for firewall")
	}
	return nil
}

func (c *Cloud) createSubnetworkIfNotExists(ctx context.Context, subnetworkName, vpcName string) error {
	client, err := compute.NewSubnetworksRESTClient(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create subnetworks client")
	}
	defer client.Close()
	subnetwork, err := client.Get(ctx, &computepb.GetSubnetworkRequest{
		Project:    c.Project,
		Region:     c.Region,
		Subnetwork: subnetworkName,
	})
	if err != nil {
		var gerr *googleapi.Error
		if ok := errors.As(err, &gerr); (ok && gerr.Code != http.StatusNotFound) || !ok {
			return errors.Wrap(err, "failed to get subnetwork")
		}
	}
	if subnetwork != nil {
		return nil
	}
	subnetwork = &computepb.Subnetwork{
		Name:                  utils.Ref(subnetworkName),
		IpCidrRange:           utils.Ref(cloud.DefaultVpcCidrBlock),
		Network:               utils.Ref(path.Join("projects", c.Project, "global", "networks", vpcName)),
		PrivateIpGoogleAccess: utils.Ref(true),
		Region:                utils.Ref(path.Join("projects", c.Project, "regions", c.Region)),
		StackType:             utils.Ref("IPV4_ONLY"),
	}
	op, err := client.Insert(ctx, &computepb.InsertSubnetworkRequest{
		Project:            c.Project,
		Region:             c.Region,
		SubnetworkResource: subnetwork,
	})
	if err != nil {
		return errors.Wrap(err, "failed to insert subnetwork")
	}
	if err = op.Wait(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for subnetwork")
	}
	return nil
}

func (c *Cloud) keyPairPath(resourceID string) (string, error) {
	cfg := c.config(resourceID)
	filePath, err := filepath.Abs(path.Join(cfg.pathToKeyPair, cfg.keyPair+".pem"))
	if err != nil {
		return "", errors.Wrap(err, "failed to get absolute key pair path")
	}
	return filePath, nil
}

func (c *Cloud) RunCommand(ctx context.Context, resourceID string, instance cloud.Instance, cmd string) (string, error) {
	sshConfig, err := c.sshConfig(resourceID)
	if err != nil {
		return "", errors.Wrap(err, "ssh config")
	}
	return utils.RunCommand(ctx, cmd, instance.PublicIpAddress, sshConfig)
}

func (c *Cloud) SendFile(ctx context.Context, resourceID string, instance cloud.Instance, file io.Reader, remotePath string) error {
	sshConfig, err := c.sshConfig(resourceID)
	if err != nil {
		return errors.Wrap(err, "ssh config")
	}
	return utils.SendFile(ctx, file, remotePath, instance.PublicIpAddress, sshConfig)
}

func (c *Cloud) EditFile(ctx context.Context, resourceID string, instance cloud.Instance, path string, editFunc func(io.ReadWriteSeeker) error) error {
	sshConfig, err := c.sshConfig(resourceID)
	if err != nil {
		return errors.Wrap(err, "ssh config")
	}
	return utils.EditFile(ctx, instance.PublicIpAddress, path, sshConfig, editFunc)
}

func (c *Cloud) Credentials() (cloud.Credentials, error) {
	// TODO
	return cloud.Credentials{}, errors.New("not implemented")
}

func (c *Cloud) DeleteInfrastructure(ctx context.Context, resourceID string) error {
	cfg := c.config(resourceID)
	list, err := c.listInstances(ctx, resourceID, nil)
	if err != nil {
		return errors.Wrap(err, "failed to list instances")
	}
	g, gCtx := errgroup.WithContext(ctx)

	for _, instance := range list {
		instance := instance
		g.Go(func() error {
			op, err := c.client.Instances.Delete(ctx, &computepb.DeleteInstanceRequest{
				Instance: instance.GetName(),
				Project:  c.Project,
				Zone:     c.Zone,
			})
			if err != nil {
				if !c.Meta.IgnoreErrorsOnDestroy {
					return errors.Wrapf(err, "delete %s instance", instance.GetName())
				} else {
					tflog.Error(gCtx, "failed to delete instance", map[string]interface{}{
						"instance": instance.GetName(), "error": err.Error(),
					})
				}
			}
			if err = op.Wait(ctx); err != nil {
				if !c.Meta.IgnoreErrorsOnDestroy {
					return errors.Wrapf(err, "delete %s instance", instance.GetName())
				} else {
					tflog.Error(gCtx, "failed to wait instance deletion", map[string]interface{}{
						"instance": instance.GetName(), "error": err.Error(),
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
		firewall := cfg.vpcName + "-allow-all"
		op, err := c.client.Firewalls.Delete(ctx, &computepb.DeleteFirewallRequest{
			Firewall: firewall,
			Project:  c.Project,
		})
		if err != nil {
			var gerr *googleapi.Error
			if ok := errors.As(err, &gerr); (ok && gerr.Code != http.StatusNotFound) || !ok {
				if !c.Meta.IgnoreErrorsOnDestroy {
					return errors.Wrap(err, "failed to delete firewall")
				}
				tflog.Error(ctx, "failed to delete firewall", map[string]interface{}{
					"firewall": firewall, "error": err.Error(),
				})
			}
		} else {
			if err := op.Wait(ctx); err != nil {
				if !c.Meta.IgnoreErrorsOnDestroy {
					return errors.Wrap(err, "failed to wait for firewall deletion")
				}
				tflog.Error(ctx, "failed to wait for firewall deletion", map[string]interface{}{
					"firewall": firewall, "error": err.Error(),
				})
			}
		}
		subnetwork := cfg.subnetwork
		op, err = c.client.Subnetworks.Delete(ctx, &computepb.DeleteSubnetworkRequest{
			Project:    c.Project,
			Region:     c.Region,
			Subnetwork: subnetwork,
		})
		if err != nil {
			var gerr *googleapi.Error
			if ok := errors.As(err, &gerr); (ok && gerr.Code != http.StatusNotFound) || !ok {
				if !c.Meta.IgnoreErrorsOnDestroy {
					return errors.Wrap(err, "failed to delete subnetwork")
				}
				tflog.Error(ctx, "failed to delete subnetwork", map[string]interface{}{
					"subnetwork": subnetwork, "error": err.Error(),
				})
			}
		} else {
			if err := op.Wait(ctx); err != nil {
				if !c.Meta.IgnoreErrorsOnDestroy {
					return errors.Wrap(err, "failed to wait for firewall deletion")
				}
				tflog.Error(ctx, "failed to wait for subnetwork deletion", map[string]interface{}{
					"subnetwork": firewall, "error": err.Error(),
				})
			}
		}
		network := cfg.vpcName
		op, err = c.client.Networks.Delete(ctx, &computepb.DeleteNetworkRequest{
			Network: network,
			Project: c.Project,
		})
		if err != nil {
			var gerr *googleapi.Error
			if ok := errors.As(err, &gerr); (ok && gerr.Code != http.StatusNotFound) || !ok {
				if !c.Meta.IgnoreErrorsOnDestroy {
					return errors.Wrap(err, "failed to delete network")
				}
				tflog.Error(ctx, "failed to delete network", map[string]interface{}{
					"network": network, "error": err.Error(),
				})
			}
		} else {
			if err := op.Wait(ctx); err != nil {
				if !c.Meta.IgnoreErrorsOnDestroy {
					return errors.Wrap(err, "failed to wait for firewall deletion")
				}
				tflog.Error(ctx, "failed to wait for network deletion", map[string]interface{}{
					"network": firewall, "error": err.Error(),
				})
			}
		}
	}

	return nil
}

const user = "ubuntu"

func (c *Cloud) sshConfig(resourceID string) (*ssh.ClientConfig, error) {
	sshKeyPath, err := c.keyPairPath(resourceID)
	if err != nil {
		return nil, errors.Wrap(err, "get key pair path")
	}
	sshConfig, err := utils.SSHConfig(user, sshKeyPath)
	if err != nil {
		return nil, errors.Wrap(err, "get ssh config")
	}
	return sshConfig, nil
}
