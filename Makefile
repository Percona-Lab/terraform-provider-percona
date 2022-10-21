.DEFAULT_GOAL := run

PROVIDER_DIR=~/.terraform.d/plugins/terraform-percona.com/terraform-percona/percona/0.9.0/linux_amd64

setup-dir:
	mkdir -p $(PROVIDER_DIR)

build:
	go build -gcflags="all=-N -l" -o terraform-provider-percona && cp terraform-provider-percona $(PROVIDER_DIR)

init-dir:
	terraform init

run:
	export TF_LOG=INFO && terraform apply -no-color 2>&1

destroy:
	terraform destroy

clean:
	rm .terraform.lock.hcl
	rm terraform.tfstate
	rm terraform-provider-percona

all: setup-dir build init-dir run
