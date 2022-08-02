package gcp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"google.golang.org/api/compute/v1"
	"os"
	"path"
	"path/filepath"
	"strings"
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
	keyPair       string
	pathToKeyPair string
	machineType   string
	publicKey     string
}

func (cloud *Cloud) Configure(resourceId string, data *schema.ResourceData) error {
	if cloud.configs == nil {
		cloud.configs = make(map[string]*resourceConfig)
	}
	if _, ok := cloud.configs[resourceId]; !ok {
		cloud.configs[resourceId] = &resourceConfig{}
	}
	cfg := cloud.configs[resourceId]
	if v, ok := data.Get(KeyPairName).(string); ok {
		cfg.keyPair = v
	}

	if v, ok := data.Get(PathToKeyPairStorage).(string); ok {
		cfg.pathToKeyPair = v
	}

	if v, ok := data.Get(MachineType).(string); ok {
		cfg.machineType = v
	}

	var err error
	cloud.client, err = compute.NewService(context.TODO())
	if err != nil {
		return errors.Wrap(err, "failed to create compute client")
	}
	return nil
}

//const sourceImage = "projects/ubuntu-os-cloud/global/images/ubuntu-minimal-2204-jammy-v20220712"
const sourceImage = "projects/ubuntu-os-cloud/global/images/ubuntu-minimal-2004-focal-v20220713"

func (cloud *Cloud) CreateInstances(resourceId string, size int64) ([]service.Instance, error) {
	cfg := cloud.configs[resourceId]
	publicKey := "ubuntu:" + cfg.publicKey
	diskType := path.Join("projects", cloud.Project, "zones", cloud.Zone, "diskTypes/pd-balanced")
	subnetwork := path.Join("projects", cloud.Project, "regions", cloud.Region, "subnetworks", "default")
	machineTypePath := path.Join("projects", cloud.Project, "zones", cloud.Zone, "machineTypes", cfg.machineType)

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
						DiskName:    name,
						DiskType:    diskType,
						SourceImage: sourceImage,
					},
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
				ClusterResourcesTagName: strings.ToLower(resourceId),
			},
			Zone: path.Join("projects", cloud.Project, "zones", cloud.Zone),
		}
		_, err := cloud.client.Instances.Insert(cloud.Project, cloud.Zone, instance).Do()
		if err != nil {
			return nil, errors.Wrap(err, "failed to insert instance")
		}
	}

	return cloud.waitUntilAllInstancesReady(resourceId, int(size))
}

func (cloud *Cloud) waitUntilAllInstancesReady(resourceId string, size int) ([]service.Instance, error) {
	for {
		time.Sleep(time.Second)
		list, err := cloud.client.Instances.List(cloud.Project, cloud.Zone).Filter("labels." + ClusterResourcesTagName + ":" + strings.ToLower(resourceId)).Do()
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
			instances := make([]service.Instance, len(list.Items))
			for i, v := range list.Items {
				instances[i] = service.Instance{
					PrivateIpAddress: v.NetworkInterfaces[0].NetworkIP,
					PublicIpAddress:  v.NetworkInterfaces[0].AccessConfigs[0].NatIP,
				}
			}
			time.Sleep(time.Second * 60)
			return instances, nil
		}
	}
}

func (cloud *Cloud) CreateInfrastructure(resourceId string) error {
	sshKeyPath, err := cloud.keyPairPath(resourceId)
	if err != nil {
		return errors.Wrap(err, "key pair path")
	}
	cfg := cloud.configs[resourceId]
	cfg.publicKey, err = createSSHKey(sshKeyPath)
	if err != nil {
		return errors.Wrap(err, "failed to create SSH key")
	}
	return nil
}

func createSSHKey(keyPath string) (string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return "", err
	}

	err = privateKey.Validate()
	if err != nil {
		return "", err
	}

	privDER := x509.MarshalPKCS1PrivateKey(privateKey)

	privBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privDER,
	}

	privatePEMbytes := pem.EncodeToMemory(&privBlock)

	publicRsaKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(keyPath, privatePEMbytes, 0400); err != nil {
		return "", err
	}

	pubKeyBytes := ssh.MarshalAuthorizedKey(publicRsaKey)
	return string(pubKeyBytes), nil
}

func (cloud *Cloud) keyPairPath(resourceId string) (string, error) {
	cfg := cloud.configs[resourceId]
	filePath, err := filepath.Abs(path.Join(cfg.pathToKeyPair, cfg.keyPair+".pem"))
	if err != nil {
		return "", errors.Wrap(err, "failed to get absolute key pair path")
	}
	return filePath, nil
}

func (cloud *Cloud) RunCommand(resourceId string, instance service.Instance, cmd string) (string, error) {
	sshKeyPath, err := cloud.keyPairPath(resourceId)
	if err != nil {
		return "", err
	}
	sshConfig, err := utils.SSHConfig("ubuntu", sshKeyPath)
	if err != nil {
		return "", errors.Wrap(err, "ssh config")
	}
	return utils.RunCommand(cmd, instance.PublicIpAddress, sshConfig)
}
func (cloud *Cloud) DeleteInfrastructure(resourceId string) error {
	list, err := cloud.client.Instances.List(cloud.Project, cloud.Zone).Filter("labels." + ClusterResourcesTagName + ":" + resourceId).Do()
	if err != nil {
		return errors.Wrap(err, "failed to list instances")
	}
	for _, v := range list.Items {
		if _, err := cloud.client.Instances.Delete(cloud.Project, cloud.Zone, v.Name).Do(); err != nil {
			return errors.Wrapf(err, "delete %s instance", v.Name)
		}
	}
	return nil
}
