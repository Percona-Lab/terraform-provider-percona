package gcp

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

type operationCall interface {
	Do(opts ...googleapi.CallOption) (*compute.Operation, error)
}

const (
	errNotFound = "not found"
)

func doUntilStatus(ctx context.Context, operation operationCall, status string) error {
	queryParameter := googleapi.QueryParameter("requestId", uuid.NewString())
	for {
		op, err := operation.Do(queryParameter)
		if err != nil {
			gerr, ok := err.(*googleapi.Error)
			if ok {
				if gerr.Code == http.StatusNotFound {
					return errors.New(errNotFound)
				}
			}
			return err
		}
		if op.Error != nil {
			data, _ := op.Error.MarshalJSON()
			return errors.New("operation error: " + string(data))
		}
		if op.Status == status {
			break
		}
		time.Sleep(time.Second)
		select {
		case <-ctx.Done():
			return nil
		default:
		}
	}
	return nil
}
