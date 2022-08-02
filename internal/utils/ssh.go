package utils

import (
	"encoding/pem"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"os"
)

func SSHConfig(user string, privateKeyPath string) (*ssh.ClientConfig, error) {
	key, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, errors.Wrap(err, "read private key")
	}
	signer, err := signerFromKey(key)
	if err != nil {
		return nil, err
	}

	return &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}, nil
}

func signerFromKey(key []byte) (ssh.Signer, error) {
	pemBlock, _ := pem.Decode(key)
	if pemBlock == nil {
		return nil, errors.New("pem decode failed")
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, errors.Wrap(err, "parse private key")
	}

	return signer, nil
}

func RunCommand(cmd string, host string, config *ssh.ClientConfig) (string, error) {
	conn, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return "", errors.Wrap(err, "ssh dial")
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		return "", errors.Wrap(err, "ssh new session")
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)

	return string(output), errors.Wrapf(err, "output %s, cmd %s", string(output), cmd)
}
