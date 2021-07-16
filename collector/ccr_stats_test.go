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
	"github.com/go-kit/kit/log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestCCRStats(t *testing.T) {

	ti := map[string]string{
		"7.7.1": `{"indices":[{"index":"myIndex1","shards":[{"remote_cluster":"leader_cluster","leader_index":"myIndex1","follower_index":"myIndex1","shard_id":0,"leader_global_checkpoint":1,"leader_max_seq_no":1,"follower_global_checkpoint":1,"follower_max_seq_no":1,"last_requested_seq_no":1,"outstanding_read_requests":1,"outstanding_write_requests":0,"write_buffer_operation_count":0,"write_buffer_size_in_bytes":0,"follower_mapping_version":1,"follower_settings_version":4,"follower_aliases_version":2,"total_read_time_millis":0,"total_read_remote_exec_time_millis":0,"successful_read_requests":0,"failed_read_requests":0,"operations_read":0,"bytes_read":0,"total_write_time_millis":0,"successful_write_requests":0,"failed_write_requests":0,"operations_written":0,"read_exceptions":[],"time_since_last_read_millis":44233},{"remote_cluster":"leader_cluster","leader_index":"myIndex1","follower_index":"myIndex1","shard_id":1,"leader_global_checkpoint":2,"leader_max_seq_no":2,"follower_global_checkpoint":2,"follower_max_seq_no":2,"last_requested_seq_no":2,"outstanding_read_requests":1,"outstanding_write_requests":0,"write_buffer_operation_count":0,"write_buffer_size_in_bytes":0,"follower_mapping_version":1,"follower_settings_version":4,"follower_aliases_version":2,"total_read_time_millis":0,"total_read_remote_exec_time_millis":0,"successful_read_requests":0,"failed_read_requests":0,"operations_read":0,"bytes_read":0,"total_write_time_millis":0,"successful_write_requests":0,"failed_write_requests":0,"operations_written":0,"read_exceptions":[],"time_since_last_read_millis":44130}]},{"index":"myIndex2","shards":[{"remote_cluster":"leader_cluster","leader_index":"myIndex2","follower_index":"myIndex2","shard_id":0,"leader_global_checkpoint":10,"leader_max_seq_no":4,"follower_global_checkpoint":4,"follower_max_seq_no":4,"last_requested_seq_no":4,"outstanding_read_requests":1,"outstanding_write_requests":0,"write_buffer_operation_count":0,"write_buffer_size_in_bytes":0,"follower_mapping_version":1,"follower_settings_version":4,"follower_aliases_version":2,"total_read_time_millis":0,"total_read_remote_exec_time_millis":0,"successful_read_requests":0,"failed_read_requests":0,"operations_read":0,"bytes_read":0,"total_write_time_millis":0,"successful_write_requests":0,"failed_write_requests":0,"operations_written":0,"read_exceptions":[],"time_since_last_read_millis":43920}]}]}`,
	}
	for ver, out := range ti {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, out)
		}))
		defer ts.Close()

		u, err := url.Parse(ts.URL)
		if err != nil {
			t.Fatalf("Failed to parse URL: %s", err)
		}
		i := NewCCRStats(log.NewNopLogger(), http.DefaultClient, u)
		stats, err := i.fetchCCRStatsResponse()
		if err != nil {
			t.Fatalf("Failed to fetch or decode CCR stats: %s", err)
		}
		t.Logf("[%s] CCR stats Response: %+v", ver, stats)
		if 2 != len(stats.IndexCCRStats) {
			t.Errorf("Number of indices is not correct. Expected statistics for two indices")
		}
		if 2 != len(stats.IndexCCRStats[0].IndexCCRStatsShards) {
			t.Errorf("Wrong number of primary shards per index [%s]", stats.IndexCCRStats[0].Index)
		}
		if 1 != stats.IndexCCRStats[0].IndexCCRStatsShards[0].LeaderGlobalCheckpoint {
			t.Errorf("Number representing global chakpont is not as expected")
		}
		if 6 != stats.IndexCCRStats[1].IndexCCRStatsShards[0].LeaderGlobalCheckpoint-stats.IndexCCRStats[1].IndexCCRStatsShards[0].FollowerGlobalCheckpoint {
			t.Errorf("Replication lag is not as expected")
		}
	}
}
