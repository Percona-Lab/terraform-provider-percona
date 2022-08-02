# AWS provider configuration
provider "percona" {
  region  = "eu-north-1"
  profile = "default"
  cloud   = "aws"
}

# GCP provider configuration
#provider "percona" {
#  region  = "europe-west1"
#  zone    = "europe-west1-c"
#  project = "project-name"
#  cloud =   "gcp"
#}

resource "percona_ps" "ps" {
  instance_type            = "t3.micro" # for AWS
  machine_type             = "e2-micro" # for GCP
  key_pair_name            = "sshKey1"
  password                 = "password"
  replica_password         = "replicaPassword"
  cluster_size             = 2
  path_to_key_pair_storage = "/tmp/"
}

resource "percona_pxc" "pxc" {
  instance_type            = "t3.micro" # for AWS
  machine_type             = "e2-micro" # for GCP
  key_pair_name            = "sshKey2"
  password                 = "password"
  cluster_size             = 2
  path_to_key_pair_storage = "/tmp/"
}