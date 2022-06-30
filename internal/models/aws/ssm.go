package aws

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/pkg/errors"
	"time"
)

func (manager *XtraDBClusterManager) RunCommand(instanceId string, cmd string) error {
	s := ssm.New(manager.Session)
	in := ssm.SendCommandInput{
		Comment:         nil,
		DocumentName:    aws.String("AWS-RunShellScript"),
		DocumentVersion: aws.String("1"),
		InstanceIds:     []*string{aws.String(instanceId)},
		Parameters:      map[string][]*string{"commands": {aws.String(cmd)}},
	}
	out, err := s.SendCommand(&in)
	if err != nil {
		return errors.Wrap(err, "failed to send command")
	}
	commandID := out.Command.CommandId
	output, err := waitCommand(s, *commandID, instanceId)
	if err != nil {
		return errors.Wrap(err, output)
	}
	return nil
}

func waitCommand(s *ssm.SSM, commandId string, instanceId string) (string, error) {
	in := &ssm.GetCommandInvocationInput{
		CommandId:  aws.String(commandId),
		InstanceId: aws.String(instanceId),
	}
	for {
		out, err := s.GetCommandInvocation(in)
		if err != nil {
			if _, ok := err.(*ssm.InvocationDoesNotExist); ok {
				time.Sleep(time.Millisecond * 100)
				continue
			}
			return "", err
		}
		switch aws.StringValue(out.Status) {
		case ssm.CommandStatusSuccess:
			return aws.StringValue(out.StandardOutputContent), nil
		case ssm.CommandStatusCancelled, ssm.CommandStatusTimedOut, ssm.CommandStatusFailed, ssm.CommandStatusCancelling:
			return "", fmt.Errorf("%s command has status %s", aws.StringValue(out.CommandId), aws.StringValue(out.Status))
		}
		time.Sleep(time.Millisecond * 100)
	}
}
