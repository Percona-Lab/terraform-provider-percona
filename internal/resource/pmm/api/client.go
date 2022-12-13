package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
)

type Client struct {
	c        *http.Client
	endpoint *url.URL
}

func NewClient(pmmEndpoint string) (*Client, error) {
	parsedEndpoint, err := url.Parse(pmmEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse pmm endpoint")
	}
	parsedEndpoint.Path = ""
	return &Client{
		endpoint: parsedEndpoint,
		c:        http.DefaultClient,
	}, nil
}

func (c *Client) Post(path string, request any) ([]byte, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal request")
	}
	resp, err := c.c.Post(c.endpoint.JoinPath(path).String(), "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("non-ok status code %d: %s", resp.StatusCode, string(respData))
	}
	return respData, nil
}

func (c *Client) RDSDiscover(AwsAccessKey, AwsSecretKey string) ([]RDSInstance, error) {
	data, err := c.Post("/v1/management/RDS/Discover", &DiscoverRDSRequest{
		AwsAccessKey: AwsAccessKey,
		AwsSecretKey: AwsSecretKey,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to send post request")
	}
	resp := DiscoverRDSResponse{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal data")
	}
	return resp.RdsInstances, nil
}

func (c *Client) RDSAdd(request *AddRDSRequest) (AddRDSResponse, error) {
	data, err := c.Post("/v1/management/RDS/Add", request)
	if err != nil {
		return AddRDSResponse{}, errors.Wrap(err, "failed to send post request")
	}
	resp := AddRDSResponse{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return AddRDSResponse{}, errors.Wrap(err, "failed to unmarshal data")
	}
	return resp, nil
}

func (c *Client) ServicesList(request *ServicesListRequest) (ServicesListResponse, error) {
	data, err := c.Post("/v1/inventory/Services/List", request)
	if err != nil {
		return ServicesListResponse{}, errors.Wrap(err, "failed to send post request")
	}
	resp := ServicesListResponse{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return ServicesListResponse{}, errors.Wrap(err, "failed to unmarshal data")
	}
	return resp, nil
}

func (c *Client) ServicesRemove(request *ServicesRemoveRequest) error {
	_, err := c.Post("/v1/inventory/Services/Remove", request)
	if err != nil {
		return errors.Wrap(err, "failed to send post request")
	}
	return nil
}
