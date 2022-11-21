package metrics_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"terraform-percona/internal/metrics"
	"terraform-percona/internal/resource"
	"terraform-percona/internal/resource/pmm"
	"terraform-percona/internal/resource/ps"
	"terraform-percona/internal/resource/pxc"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestTelemetryMarshal(t *testing.T) {
	tests := []struct {
		id       string
		version  string
		resource resource.Resource
		time     time.Time
	}{
		{"id", "test-ver", &ps.PerconaServer{}, time.Now()},
		{"id", "test-ver", &pxc.PerconaXtraDBCluster{}, time.Now().Local()},
		{"id", "test-ver", &pmm.PMM{}, time.Now()},
	}
	for _, tt := range tests {
		tt := tt
		resData := defaultDataFromResource(tt.resource, false)
		resData.SetId(tt.id)
		telemetry := metrics.NewTelemetry(tt.version, tt.resource.Name(), tt.resource.Schema(), tt.time, resData)
		data, err := json.Marshal(telemetry)
		if err != nil {
			t.Fatal(err)
		}
		result := map[string]any{}
		err = json.Unmarshal(data, &result)
		if err != nil {
			t.Fatal(err)
		}
		expected := map[string]any{
			"id":                   tt.id,
			"time":                 tt.time.UTC().Format(time.RFC3339Nano),
			"pmmServerTelemetryId": "xxxxxxxxxxxxxxxxxxxxxx",
			"pmmServerVersion":     tt.version,
			"upDuration":           "0.673692200s",
			"distributionMethod":   "DOCKER",
			"metrics": []any{
				map[string]any{
					"key":   "product",
					"value": "terraform-provider",
				},
				map[string]any{
					"key":   "resource",
					"value": tt.resource.Name(),
				},
			},
		}
		for k, v := range expected {
			resultValue, ok := result["metrics"].(map[string]any)[k]
			if !ok {
				t.Errorf("expected key %s doesn't exist", k)
				continue
			}
			if k == "metrics" {
				if !reflect.DeepEqual(resultValue, v) {
					t.Errorf("invalid metrics: got %v, want %v", resultValue, v)
				}
				continue
			}
			if resultValue != v {
				t.Errorf("key %s: expected value %s, got %s", k, v, resultValue)
				continue
			}
		}
	}
}

func TestMetrics(t *testing.T) {
	tests := []struct {
		resource resource.Resource
		expected []metrics.Metric
	}{
		{new(ps.PerconaServer), []metrics.Metric{
			{Key: "product", Value: "terraform-provider"},
			{Key: "resource", Value: "ps"},
			{Key: "version", Value: "somestring"},
			{Key: "cluster_size", Value: "3"},
			{Key: "volume_type", Value: "somestring"},
			{Key: "volume_size", Value: "20"},
			{Key: "vpc_id", Value: "somestring"},
			{Key: "config_file_path", Value: "somestring"},
			{Key: "instance_type", Value: "somestring"},
			{Key: "volume_iops", Value: "1234"},
			{Key: "port", Value: "3306"},
			{Key: "vpc_name", Value: "somestring"},
			{Key: "key_pair_name", Value: "somestring"},
			{Key: "pmm_address", Value: "somestring"},
			{Key: "volume_throughput", Value: "1234"},
		}},
		{new(pxc.PerconaXtraDBCluster), []metrics.Metric{
			{Key: "product", Value: "terraform-provider"},
			{Key: "resource", Value: "pxc"},
			{Key: "version", Value: "somestring"},
			{Key: "cluster_size", Value: "3"},
			{Key: "volume_type", Value: "somestring"},
			{Key: "volume_size", Value: "20"},
			{Key: "vpc_id", Value: "somestring"},
			{Key: "config_file_path", Value: "somestring"},
			{Key: "instance_type", Value: "somestring"},
			{Key: "volume_iops", Value: "1234"},
			{Key: "port", Value: "3306"},
			{Key: "vpc_name", Value: "somestring"},
			{Key: "pmm_address", Value: "somestring"},
			{Key: "galera_port", Value: "4567"},
			{Key: "key_pair_name", Value: "somestring"},
			{Key: "volume_throughput", Value: "1234"},
		}},
		{new(pmm.PMM), []metrics.Metric{
			{Key: "product", Value: "terraform-provider"},
			{Key: "resource", Value: "pmm"},
			{Key: "vpc_name", Value: "somestring"},
			{Key: "key_pair_name", Value: "somestring"},
			{Key: "volume_type", Value: "somestring"},
			{Key: "volume_size", Value: "20"},
			{Key: "vpc_id", Value: "somestring"},
			{Key: "instance_type", Value: "somestring"},
			{Key: "volume_iops", Value: "1234"},
			{Key: "volume_throughput", Value: "1234"},
		}},
	}
	for _, tt := range tests {
		resData := defaultDataFromResource(tt.resource, true)
		telemetry := metrics.NewTelemetry("versionn", tt.resource.Name(), tt.resource.Schema(), time.Now(), resData)
		if len(telemetry.Metrics.Metrics) != len(tt.expected) {
			t.Errorf("invalid metrics length len(got) = %d, len(want) = %d", len(telemetry.Metrics.Metrics), len(tt.expected))
		}
		expectedMap := make(map[string]string)
		for _, v := range telemetry.Metrics.Metrics {
			expectedMap[v.Key] = v.Value
		}
		gMap := make(map[string]string)
		for _, v := range tt.expected {
			gMap[v.Key] = v.Value
		}
		for eKey, eValue := range expectedMap {
			gotValue, ok := gMap[eKey]
			if !ok {
				t.Error(tt.resource.Name(), fmt.Sprintf("expected key %s", eKey))
				continue
			}
			if eValue != gotValue {
				t.Error(tt.resource.Name(), fmt.Sprintf("key %s: expected value %s, got %s", eKey, eValue, gotValue))
			}
		}
		for gKey, gValue := range gMap {
			eValue, ok := expectedMap[gKey]
			if !ok {
				t.Error(tt.resource.Name(), fmt.Sprintf("got key %s", gKey))
				continue
			}
			if gValue != eValue {
				t.Error(tt.resource.Name(), fmt.Sprintf("key %s: got value %s, expected %s", gKey, gValue, eValue))
			}
		}
	}
}

func defaultDataFromResource(res resource.Resource, fill bool) *schema.ResourceData {
	m := resource.ResourcesMap(res)
	for _, v := range m {
		resdata := v.TestResourceData()
		if fill {
			for k, v := range res.Schema() {
				if v.Default != nil {
					resdata.Set(k, v.Default)
				} else {
					switch v.Type {
					case schema.TypeBool:
						resdata.Set(k, true)
					case schema.TypeInt:
						resdata.Set(k, 1234)
					case schema.TypeFloat:
						resdata.Set(k, 1234.56)
					case schema.TypeString:
						resdata.Set(k, "somestring")
					case schema.TypeList:
						resdata.Set(k, []int{1, 2, 3, 4})
					case schema.TypeMap:
						resdata.Set(k, map[string]interface{}{"1": 2})
						//case schema.TypeSet:
					}
				}
			}
		}
		return resdata
	}
	return nil
}
