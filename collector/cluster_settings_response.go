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

// ClusterSettingsFullResponse is a representation of a Elasticsearch Cluster Settings
type ClusterSettingsFullResponse struct {
	Defaults   ClusterSettingsResponse `json:"defaults"`
	Persistent ClusterSettingsResponse `json:"persistent"`
	Transient  ClusterSettingsResponse `json:"transient"`
}

// ClusterSettingsResponse is a representation of a Elasticsearch Cluster Settings
type ClusterSettingsResponse struct {
	Cluster Cluster `json:"cluster"`
}

// Cluster is a representation of a Elasticsearch Cluster Settings
type Cluster struct {
	Routing          Routing `json:"routing"`
	MaxShardsPerNode string  `json:"max_shards_per_node"`
	Name             string  `json:"name"`
}

// Routing is a representation of a Elasticsearch Cluster shard routing configuration
type Routing struct {
	Allocation Allocation `json:"allocation"`
}

// Allocation is a representation of a Elasticsearch Cluster shard routing allocation settings
type Allocation struct {
	Enabled string `json:"enable"`
}
