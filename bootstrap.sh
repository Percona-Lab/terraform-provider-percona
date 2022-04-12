#!/bin/bash

apt-get update
apt-get install net-tools

CONFIG_PATH="/etc/mysql/mysql.conf.d/mysqld.cnf"
FIRST_NODE_ADDRESS="10.0.1.1"
SECOND_NODE_ADDRESS="10.0.1.2"
THIRD_NODE_ADDRESS="10.0.1.3"
PRIVATE_NODE_ADDRESS=$(ifconfig | grep -Eo 'inet (addr:)?([0-9]*\.){3}[0-9]*' | grep -Eo '([0-9]*\.){3}[0-9]*' | grep -v '127.0.0.1')
MYSQL_SELECTION_DEFAULT_AUTH_OVVERIDE="select Use Strong Password Encryption (RECOMMENDED)"

SetupPerconaXtraDBCluster() {
  InstallFromRepository
  ConfigureMysqldCnf
  StartMysql
}

InstallFromRepository() {
  apt-get install debconf-utils
  apt-get install -y wget gnupg2 lsb-release curl
  wget https://repo.percona.com/apt/percona-release_latest.generic_all.deb
  dpkg -i percona-release_latest.generic_all.deb
  apt-get update
  percona-release setup pxc80

  #for silent mysql installation
  echo "percona-xtradb-cluster-server   percona-xtradb-cluster-server/re-root-pass password pass" | debconf-set-selections
  echo "percona-xtradb-cluster-server   percona-xtradb-cluster-server/root-pass password pass" | debconf-set-selections
  echo "percona-xtradb-cluster-server   percona-xtradb-cluster-server/default-auth-override ${MYSQL_SELECTION_DEFAULT_AUTH_OVVERIDE}" | debconf-set-selections

  DEBIAN_FRONTEND=noninteractive apt-get install -y percona-xtradb-cluster
}

ConfigureMysqldCnf() {
  sed -i "s/^wsrep_cluster_address=.*/wsrep_cluster_address=gcomm:\/\/${FIRST_NODE_ADDRESS},${SECOND_NODE_ADDRESS},${THIRD_NODE_ADDRESS}/" $CONFIG_PATH
  sed -i "s/^wsrep_node_name=.*/wsrep_node_name=${PRIVATE_NODE_ADDRESS}/" $CONFIG_PATH
  sed -i "s/^#wsrep_node_address=.*/wsrep_node_address=${PRIVATE_NODE_ADDRESS}/" $CONFIG_PATH

  if grep -q "pxc-encrypt-cluster-traffic" $CONFIG_PATH; then
    :
  else
    echo "pxc-encrypt-cluster-traffic=OFF" | tee -a $CONFIG_PATH
  fi
}

StartMysql() {
  if [[ "$FIRST_NODE_ADDRESS" == "$PRIVATE_NODE_ADDRESS" ]]; then
    systemctl start mysql@bootstrap.service
  elif [[ "$SECOND_NODE_ADDRESS" == "$PRIVATE_NODE_ADDRESS" ]] || [[ "$THIRD_NODE_ADDRESS" == "$PRIVATE_NODE_ADDRESS" ]]; then
    systemctl start mysql
  else
    echo "unexpected private ip - ${PRIVATE_NODE_ADDRESS}"
  fi
}

SetupPerconaXtraDBCluster
