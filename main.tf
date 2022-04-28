provider "percona" {
  region  = "eu-north-1"
  profile = "default"
}

resource "percona_cluster" "pxc" {
  instance_type            = "t3.micro"
  path_to_bootstrap_script = "./bootstrap.sh"
  key_pair_name            = "sshKey"
  cluster_size             = 3
  min_count                = 1
  max_count                = 1
  path_to_key_pair_storage = "/tmp/"
}