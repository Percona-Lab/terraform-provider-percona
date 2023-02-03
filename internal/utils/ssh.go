package utils

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"os"

	"github.com/pkg/errors"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
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

func RunCommand(ctx context.Context, cmd string, host string, config *ssh.ClientConfig) (string, error) {
	conn, err := sshDialWithContext(ctx, "tcp", host+":22", config)
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

func SSHPing(ctx context.Context, host string, config *ssh.ClientConfig) error {
	conn, err := sshDialWithContext(ctx, "tcp", host+":22", config)
	if err != nil {
		return errors.Wrap(err, "ssh dial")
	}
	return errors.Wrap(conn.Close(), "connection close")
}

func sshDialWithContext(ctx context.Context, network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	d := net.Dialer{Timeout: config.Timeout}
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, errors.Wrap(err, "dial context")
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		return nil, errors.Wrap(err, "new client conn")
	}
	return ssh.NewClient(c, chans, reqs), nil
}

func SendFile(ctx context.Context, src io.Reader, dstPath, host string, cfg *ssh.ClientConfig) error {
	conn, err := sshDialWithContext(ctx, "tcp", host+":22", cfg)
	if err != nil {
		return errors.Wrap(err, "ssh dial")
	}
	defer conn.Close()

	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		return errors.Wrap(err, "failed to create sftp client")
	}
	defer sftpClient.Close()

	dstFile, err := sftpClient.Create(dstPath)
	if err != nil {
		return errors.Wrap(err, "failed to create destination file")
	}
	defer dstFile.Close()

	if _, err = dstFile.ReadFrom(src); err != nil {
		return errors.Wrap(err, "failed to copy file")
	}
	return nil
}

func EditFile(ctx context.Context, host, path string, cfg *ssh.ClientConfig, editFunc func(io.ReadWriteSeeker) error) error {
	conn, err := sshDialWithContext(ctx, "tcp", host+":22", cfg)
	if err != nil {
		return errors.Wrap(err, "ssh dial")
	}
	defer conn.Close()

	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		return errors.Wrap(err, "failed to create sftp client")
	}
	defer sftpClient.Close()

	f, err := sftpClient.OpenFile(path, os.O_RDWR)
	if err != nil {
		return errors.Wrap(err, "open file via sftp")
	}
	defer f.Close()
	if err := editFunc(f); err != nil {
		return errors.Wrap(err, "failed to edit file")
	}

	return nil
}

func createPrivateKey(keyPath string) (*rsa.PrivateKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate private key")
	}

	err = privateKey.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate private key")
	}

	pemBlock := &pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   x509.MarshalPKCS1PrivateKey(privateKey),
	}

	if err = os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0600); err != nil {
		return nil, errors.Wrap(err, "failed to write private key")
	}
	return privateKey, nil
}

func GetSSHPublicKey(keyPath string) (string, error) {
	var privateKey *rsa.PrivateKey
	if _, err := os.Stat(keyPath); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		privateKey, err = createPrivateKey(keyPath)
		if err != nil {
			return "", errors.Wrap(err, "failed to create private key")
		}
	}
	if privateKey == nil {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return "", errors.Wrap(err, "failed to read private key file")
		}
		block, _ := pem.Decode(data)
		if block == nil {
			return "", errors.New("pem decode failed")
		}
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return "", errors.Wrap(err, "failed to parse private key")
		}
	}

	publicRsaKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", errors.Wrap(err, "failed to retrieve public key")
	}

	return string(ssh.MarshalAuthorizedKey(publicRsaKey)), nil
}
