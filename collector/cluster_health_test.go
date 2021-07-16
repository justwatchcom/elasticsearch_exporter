// Copyright 2021 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"

	"github.com/go-kit/kit/log"
)

func TestClusterHealth(t *testing.T) {
	// Testcases created using:
	//  docker run -d -p 9200:9200 elasticsearch:VERSION-alpine
	//  curl -XPUT http://localhost:9200/twitter
	//  curl http://localhost:9200/_cluster/health
	tcs := map[string]string{
		"1.7.6": `{"cluster_name":"elasticsearch","status":"yellow","timed_out":false,"number_of_nodes":1,"number_of_data_nodes":1,"active_primary_shards":5,"active_shards":5,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":5,"delayed_unassigned_shards":0,"number_of_pending_tasks":0,"number_of_in_flight_fetch":0}`,
		"2.4.5": `{"cluster_name":"elasticsearch","status":"yellow","timed_out":false,"number_of_nodes":1,"number_of_data_nodes":1,"active_primary_shards":5,"active_shards":5,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":5,"delayed_unassigned_shards":0,"number_of_pending_tasks":0,"number_of_in_flight_fetch":0,"task_max_waiting_in_queue_millis":12,"active_shards_percent_as_number":50.0}`,
		"5.4.2": `{"cluster_name":"elasticsearch","status":"yellow","timed_out":false,"number_of_nodes":1,"number_of_data_nodes":1,"active_primary_shards":5,"active_shards":5,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":5,"delayed_unassigned_shards":0,"number_of_pending_tasks":0,"number_of_in_flight_fetch":0,"task_max_waiting_in_queue_millis":12,"active_shards_percent_as_number":50.0}`,
	}
	for ver, out := range tcs {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, out)
		}))
		defer ts.Close()

		u, err := url.Parse(ts.URL)
		if err != nil {
			t.Fatalf("Failed to parse URL: %s", err)
		}
		c := NewClusterHealth(log.NewNopLogger(), http.DefaultClient, u)
		chr, err := c.fetchAndDecodeClusterHealth()
		if err != nil {
			t.Fatalf("Failed to fetch or decode cluster health: %s", err)
		}
		t.Logf("[%s] Cluster Health Response: %+v", ver, chr)
		if chr.ClusterName != "elasticsearch" {
			t.Errorf("Invalid cluster health response")
		}
		if chr.Status != "yellow" {
			t.Errorf("Invalid cluster status")
		}
		if chr.TimedOut {
			t.Errorf("Check didn't time out")
		}
		if chr.NumberOfNodes != 1 {
			t.Errorf("Wrong number of nodes")
		}
		if chr.NumberOfDataNodes != 1 {
			t.Errorf("Wrong number of data nodes")
		}
		if ver != "1.7.6" {
			if chr.TaskMaxWaitingInQueueMillis != 12 {
				t.Errorf("Wrong task max waiting time in millis")
			}
		}
	}
}

func Test_parseClusterHealth(t *testing.T) {
	desired := clusterHealthResponse{
		ClusterName: "elasticsearch",
		Status:      "yellow",

		NumberOfNodes:               1,
		NumberOfDataNodes:           1,
		ActivePrimaryShards:         30,
		ActiveShards:                30,
		UnassignedShards:            30,
		TaskMaxWaitingInQueueMillis: 12,
		ActiveShardsPercentAsNumber: 50,
	}
	for _, ver := range testElasticsearchVersions {
		t.Run(fmt.Sprintf("version %s", ver), func(t *testing.T) {
			file, err := os.Open(fmt.Sprintf("../fixtures/clusterhealth/%s.json", ver))
			if err != nil {
				t.Fatalf("failed to open fixture file: %v", err)
			}
			defer file.Close()

			got, err := parseClusterHealth(file)
			if err != nil {
				t.Errorf("parseClusterHealth() error = %v", err)
				return
			}
			if !reflect.DeepEqual(got, desired) {
				t.Errorf("parseClusterHealth() = %v, want %v", got, desired)
			}
		})
	}
}
