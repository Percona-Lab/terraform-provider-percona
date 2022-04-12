.DEFAULT_GOAL := run

PROVIDER_DIR=~/.terraform.d/plugins/terraform-percona.com/terraform-percona/percona/1.0.0/linux_amd64

setup-dir:
	mkdir -p $(PROVIDER_DIR)

build:
	go build -o terraform-provider-percona && cp terraform-provider-percona $(PROVIDER_DIR)

init-dir:
	terraform init

run:
	terraform apply

all: setup-dir build init-dir run