package gcp

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

const user = "ubuntu"
const sourceImage = "projects/ubuntu-os-cloud/global/images/ubuntu-minimal-2004-focal-v20220713"

func Schema() map[string]*schema.Schema {
	return map[string]*schema.Schema{}
}
