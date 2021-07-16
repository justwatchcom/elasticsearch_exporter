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

// CCRStatsResponse is a representation of the CCR stats
type CCRStatsResponse struct {
	IndexCCRStats []IndexCCRStatsResponse `json:"indices"`
}

// IndexCCRStatsResponse is a representation of the CCR stats per index
type IndexCCRStatsResponse struct {
	// The name of the follower index.
	Index string `json:"index"`
	// An array of shard-level following task statistics.
	IndexCCRStatsShards []IndexCCRStatsShardResponse `json:"shards"`
}

type ReadException struct {
	Exception []interface{} `json:"exception"`
	FromSeqNo int64         `json:"from_seq_no"`
	Retries   int64         `json:"retries"`
}

// IndexCCRStatsShardResponse defines CCR statistics information for shard
type IndexCCRStatsShardResponse struct {
	RemoteCluster                 string          `json:"remote_cluster"`
	LeaderIndex                   string          `json:"leader_index"`
	FollowerIndex                 string          `json:"follower_index"`
	ShardID                       int64           `json:"shard_id"`
	LeaderGlobalCheckpoint        int64           `json:"leader_global_checkpoint"`
	LeaderMaxSeqNo                int64           `json:"leader_max_seq_no"`
	FollowerGlobalCheckpoint      int64           `json:"follower_global_checkpoint"`
	FollowerMaxSeqNo              int64           `json:"follower_max_seq_no"`
	LastRequestedSeqNo            int64           `json:"last_requested_seq_no"`
	OutstandingReadRequests       int64           `json:"outstanding_read_requests"`
	OutstandingWriteRequests      int64           `json:"outstanding_write_requests"`
	WriteBufferOperationCount     int64           `json:"write_buffer_operation_count"`
	WriteBufferSizeInBytes        int64           `json:"write_buffer_size_in_bytes"`
	FollowerMappingVersion        int64           `json:"follower_mapping_version"`
	FollowerSettingsVersion       int64           `json:"follower_settings_version"`
	FollowerAliasesVersion        int64           `json:"follower_aliases_version"`
	TotalReadTimeMillis           int64           `json:"total_read_time_millis"`
	TotalReadRemoteExecTimeMillis int64           `json:"total_read_remote_exec_time_millis"`
	SuccessfulReadRequests        int64           `json:"successful_read_requests"`
	FailedReadRequests            int64           `json:"failed_read_requests"`
	OperationsRead                int64           `json:"operations_read"`
	BytesRead                     int64           `json:"bytes_read"`
	TotalWriteTimeMillis          int64           `json:"total_write_time_millis"`
	SuccessfulWriteRequests       int64           `json:"successful_write_requests"`
	FailedWriteRequests           int64           `json:"failed_write_requests"`
	OperationsWritten             int64           `json:"operations_written"`
	ReadExceptions                []ReadException `json:"read_exceptions"`
	TimeSinceListReadMillis       int64           `json:"time_since_last_read_millis"`
}
