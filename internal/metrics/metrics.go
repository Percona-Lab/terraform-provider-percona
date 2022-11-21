package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"terraform-percona/internal/version"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
)

type Metric struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type MetricsDuration time.Duration

func (t MetricsDuration) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%.9fs"`, time.Duration(t).Seconds())), nil
}

type MetricsTime time.Time

func (t MetricsTime) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, time.Time(t).UTC().Format(time.RFC3339Nano))), nil
}

type TelemetryMetrics struct {
	ID                 string          `json:"id"`
	Time               MetricsTime     `json:"time"`
	TelemetryID        string          `json:"pmmServerTelemetryId"`
	ServerVersion      string          `json:"pmmServerVersion"`
	UptimeDuration     MetricsDuration `json:"upDuration"`
	DistributionMethod string          `json:"distributionMethod"`
	Metrics            []Metric        `json:"metrics"`
}

type Telemetry struct {
	Metrics TelemetryMetrics `json:"metrics"`
}

const endpoint = "https://check.percona.com/v1/telemetry/Report"

func SendTelemetry(name string, schema map[string]*schema.Schema, data *schema.ResourceData) error {
	obj := NewTelemetry(version.Version, name, schema, time.Now(), data)
	telemetryData, err := json.Marshal(obj)
	if err != nil {
		return errors.Wrap(err, "failed to marshal data")
	}
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(telemetryData))
	if err != nil {
		return errors.Wrap(err, "failed to post request")
	}
	if resp.StatusCode != http.StatusOK {
		var responseData []byte
		_, err := resp.Body.Read(responseData)
		if err != nil {
			return errors.Wrapf(err, "non-ok status code %d: failed to read response body", resp.StatusCode)
		}
		return errors.Errorf("non-ok status code %d: %s", resp.StatusCode, string(responseData))
	}
	defer resp.Body.Close()
	return nil
}

func NewTelemetry(version string, resourceName string, schema map[string]*schema.Schema, currentTime time.Time, data *schema.ResourceData) Telemetry {
	metrics := []Metric{
		{
			Key:   "product",
			Value: "terraform-provider",
		},
		{
			Key:   "resource",
			Value: resourceName,
		},
	}
	if data != nil {
		for k, v := range schema {
			if !v.Sensitive && !v.Computed {
				val, ok := data.GetOk(k)
				if ok {
					metrics = append(metrics, Metric{
						Key:   k,
						Value: fmt.Sprint(val),
					})
				}
			}
		}
	}
	return Telemetry{
		Metrics: TelemetryMetrics{
			ID:                 data.Id(),
			Time:               MetricsTime(currentTime),
			TelemetryID:        "xxxxxxxxxxxxxxxxxxxxxx",
			ServerVersion:      version,
			UptimeDuration:     MetricsDuration(time.Nanosecond * 673692200),
			DistributionMethod: "DOCKER",
			Metrics:            metrics,
		},
	}
}
