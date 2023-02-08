package cmd

import (
	"fmt"
	"terraform-percona/internal/db"
)

func Restart() string {
	return "sudo systemctl restart mysql"
}

func RetrieveVersions() string {
	return `apt-cache show percona-server-server | grep 'Version' | sed 's/Version: //'`
}

func Init() string {
	return `
		#!/usr/bin/env bash

		set -o errexit

		sudo apt-get update
		sudo apt-get upgrade -y

		sudo mkdir /opt/percona
		sudo chown ubuntu /opt/percona
	`
}

func InstallOrchestrator() string {
	version := "3.2.6"
	return fmt.Sprintf(`
		#!/usr/bin/env bash

		set -o errexit

		sudo apt-get update
		sudo apt-get upgrade -y
		sudo apt-get install -y libonig5 libjq1 jq
		curl --fail -L https://github.com/openark/orchestrator/releases/download/v%s/orchestrator_%s_amd64.deb -o orchestrator.deb
		sudo dpkg -i orchestrator.deb
		rm orchestrator.deb

		sudo mkdir /etc/mysql
		sudo mkdir /var/lib/orchestrator
	`, version, version)
}

func InstallOrchestratorClient() string {
	version := "3.2.6"
	return fmt.Sprintf(`
		#!/usr/bin/env bash

		set -o errexit

		sudo apt-get update
		sudo apt-get install -y libonig5 libjq1 jq
		curl --fail -L https://github.com/openark/orchestrator/releases/download/v%s/orchestrator-client_%s_amd64.deb -o orchestrator-client.deb
		sudo dpkg -i orchestrator-client.deb
		rm orchestrator-client.deb
	`, version, version)
}

func InstallPerconaServer(password, version string, port int) string {
	return fmt.Sprintf(`
	#!/usr/bin/env bash

	set -o errexit

	DEBIAN_FRONTEND=noninteractive sudo -E bash -c 'apt-get install -y percona-server-client=%s percona-server-common=%s percona-server-server=%s'
	mysql -uroot -p%s -e "RENAME USER 'root'@'localhost' TO 'root'@'%%';FLUSH PRIVILEGES;"
	`, version, version, version, password)
}

func Configure(password string) string {
	return fmt.Sprintf(`
	#!/usr/bin/env bash

	set -o errexit

	sudo apt-get update
	sudo apt-get upgrade -y
	sudo apt-get install -y gnupg2 curl debconf-utils net-tools

	wget https://repo.percona.com/apt/percona-release_latest.$(lsb_release -sc)_all.deb
	sudo dpkg -i percona-release_latest.$(lsb_release -sc)_all.deb

	sudo apt-get update
	sudo percona-release setup ps80
	export MYSQL_SELECTION_DEFAULT_AUTH_OVERRIDE="select Use Strong Password Encryption (RECOMMENDED)"
	echo "percona-server-server   percona-server-server/re-root-pass password %s" | sudo debconf-set-selections
	echo "percona-server-server   percona-server-server/root-pass password %s" | sudo debconf-set-selections
	echo "percona-server-server   percona-server-server/default-auth-override ${MYSQL_SELECTION_DEFAULT_AUTH_OVERRIDE}" | sudo debconf-set-selections
	`, password, password)
}

func InstallMyRocks(password, version string) string {
	return fmt.Sprintf(`
	#!/usr/bin/env bash

	set -o errexit

	sudo apt-get install -y percona-server-rocksdb=%s
	sudo ps-admin --enable-rocksdb -uroot -p%s`, version, password)
}

func InstallPMMClient(addr string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash

	set -o errexit

	sudo percona-release disable all
	sudo percona-release enable original release
	sudo apt update
	sudo apt install -y pmm2-client
	sudo pmm-admin config --server-insecure-tls --server-url="%s"`, addr)
}

func AddServiceToPMM(password string, port int) string {
	return fmt.Sprintf(`pmm-admin add mysql --query-source=slowlog --username="%s" --password="%s" --port=%d`, db.UserPMM, password, port)
}
