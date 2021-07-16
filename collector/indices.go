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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus-community/elasticsearch_exporter/pkg/clusterinfo"
	"github.com/prometheus/client_golang/prometheus"
)

type labels struct {
	keys   func(...string) []string
	values func(*clusterinfo.Response, ...string) []string
}

type indexMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(indexStats IndexStatsIndexResponse) float64
	Labels labels
}

type shardMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(data IndexStatsIndexShardsDetailResponse) float64
	Labels labels
}

// Indices information struct
type Indices struct {
	logger          log.Logger
	client          *http.Client
	url             *url.URL
	shards          bool
	clusterInfoCh   chan *clusterinfo.Response
	lastClusterInfo *clusterinfo.Response

	up                prometheus.Gauge
	totalScrapes      prometheus.Counter
	jsonParseFailures prometheus.Counter

	indexMetrics []*indexMetric
	shardMetrics []*shardMetric
}

// NewIndices defines Indices Prometheus metrics
func NewIndices(logger log.Logger, client *http.Client, url *url.URL, shards bool) *Indices {

	indexLabels := labels{
		keys: func(...string) []string {
			return []string{"index", "cluster"}
		},
		values: func(lastClusterinfo *clusterinfo.Response, s ...string) []string {
			if lastClusterinfo != nil {
				return append(s, lastClusterinfo.ClusterName)
			}
			// this shouldn't happen, as the clusterinfo Retriever has a blocking
			// Run method. It blocks until the first clusterinfo call has succeeded
			return append(s, "unknown_cluster")
		},
	}

	shardLabels := labels{
		keys: func(...string) []string {
			return []string{"index", "shard", "node", "primary", "cluster"}
		},
		values: func(lastClusterinfo *clusterinfo.Response, s ...string) []string {
			if lastClusterinfo != nil {
				return append(s, lastClusterinfo.ClusterName)
			}
			// this shouldn't happen, as the clusterinfo Retriever has a blocking
			// Run method. It blocks until the first clusterinfo call has succeeded
			return append(s, "unknown_cluster")
		},
	}

	indices := &Indices{
		logger:        logger,
		client:        client,
		url:           url,
		shards:        shards,
		clusterInfoCh: make(chan *clusterinfo.Response),
		lastClusterInfo: &clusterinfo.Response{
			ClusterName: "unknown_cluster",
		},

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "index_stats", "up"),
			Help: "Was the last scrape of the ElasticSearch index endpoint successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "index_stats", "total_scrapes"),
			Help: "Current total ElasticSearch index scrapes.",
		}),
		jsonParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "index_stats", "json_parse_failures"),
			Help: "Number of errors while parsing JSON.",
		}),

		indexMetrics: []*indexMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "docs_primary"),
					"Count of documents with only primary shards",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Docs.Count)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "deleted_docs_primary"),
					"Count of deleted documents with only primary shards",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Docs.Deleted)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "docs_total"),
					"Total count of documents",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Docs.Count)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "deleted_docs_total"),
					"Total count of deleted documents",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Docs.Deleted)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "store_size_bytes_primary"),
					"Current total size of stored index data in bytes with only primary shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Store.SizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "store_size_bytes_total"),
					"Current total size of stored index data in bytes with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Store.SizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_count_primary"),
					"Current number of segments with only primary shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.Count)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_count_total"),
					"Current number of segments with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.Count)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_memory_bytes_primary"),
					"Current size of segments with only primary shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.MemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_memory_bytes_total"),
					"Current size of segments with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.MemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_terms_memory_primary"),
					"Current size of terms with only primary shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.TermsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_terms_memory_total"),
					"Current number of terms with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.TermsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_fields_memory_bytes_primary"),
					"Current size of fields with only primary shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.StoredFieldsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_fields_memory_bytes_total"),
					"Current size of fields with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.StoredFieldsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_term_vectors_memory_primary_bytes"),
					"Current size of term vectors with only primary shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.TermVectorsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_term_vectors_memory_total_bytes"),
					"Current size of term vectors with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.TermVectorsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_norms_memory_bytes_primary"),
					"Current size of norms with only primary shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.NormsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_norms_memory_bytes_total"),
					"Current size of norms with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.NormsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_points_memory_bytes_primary"),
					"Current size of points with only primary shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.PointsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_points_memory_bytes_total"),
					"Current size of points with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.PointsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_doc_values_memory_bytes_primary"),
					"Current size of doc values with only primary shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.DocValuesMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_doc_values_memory_bytes_total"),
					"Current size of doc values with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.DocValuesMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_index_writer_memory_bytes_primary"),
					"Current size of index writer with only primary shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.IndexWriterMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_index_writer_memory_bytes_total"),
					"Current size of index writer with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.IndexWriterMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_version_map_memory_bytes_primary"),
					"Current size of version map with only primary shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.VersionMapMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_version_map_memory_bytes_total"),
					"Current size of version map with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.VersionMapMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_fixed_bit_set_memory_bytes_primary"),
					"Current size of fixed bit with only primary shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.FixedBitSetMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segment_fixed_bit_set_memory_bytes_total"),
					"Current size of fixed bit with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.FixedBitSetMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "completion_bytes_primary"),
					"Current size of completion with only primary shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Completion.SizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "completion_bytes_total"),
					"Current size of completion with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Completion.SizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "search_open_contexts"),
					"Total number of open search contexts",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.OpenContexts)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "search_query_time_seconds_total"),
					"Total search query time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.QueryTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "search_query_total"),
					"Total number of queries",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.QueryTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "search_fetch_time_seconds_total"),
					"Total search fetch time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.FetchTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "search_fetch_total"),
					"Total search fetch count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.FetchTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "search_scroll_time_seconds_total"),
					"Total search scroll time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.ScrollTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "search_scroll_current"),
					"Current search scroll count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.ScrollCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "search_scroll_total"),
					"Total search scroll count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.ScrollTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "search_suggest_time_seconds_total"),
					"Total search suggest time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.SuggestTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "search_suggest_total"),
					"Total search suggest count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.SuggestTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "indexing_index_time_seconds_total"),
					"Total indexing index time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.IndexTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "indexing_index_total"),
					"Total indexing index count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.IndexTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "indexing_delete_time_seconds_total"),
					"Total indexing delete time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.DeleteTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "indexing_delete_total"),
					"Total indexing delete count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.DeleteTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "indexing_noop_update_total"),
					"Total indexing no-op update count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.NoopUpdateTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "indexing_throttle_time_seconds_total"),
					"Total indexing throttle time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.ThrottleTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "get_time_seconds_total"),
					"Total get time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Get.TimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "get_total"),
					"Total get count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Get.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "merge_time_seconds_total"),
					"Total merge time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.TotalTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "merge_total"),
					"Total merge count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "merge_throttle_time_seconds_total"),
					"Total merge I/O throttle time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.TotalThrottledTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "merge_stopped_time_seconds_total"),
					"Total large merge stopped time in seconds, allowing smaller merges to complete",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.TotalStoppedTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "merge_auto_throttle_bytes_total"),
					"Total bytes that were auto-throttled during merging",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.TotalAutoThrottleInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "refresh_time_seconds_total"),
					"Total refresh time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Refresh.TotalTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "refresh_total"),
					"Total refresh count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Refresh.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "flush_time_seconds_total"),
					"Total flush time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Flush.TotalTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "flush_total"),
					"Total flush count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Flush.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "warmer_time_seconds_total"),
					"Total warmer time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Warmer.TotalTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "warmer_total"),
					"Total warmer count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Warmer.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "query_cache_memory_bytes_total"),
					"Total query cache memory bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.MemorySizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "query_cache_size"),
					"Total query cache size",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.CacheSize)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "query_cache_hits_total"),
					"Total query cache hits count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.HitCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "query_cache_misses_total"),
					"Total query cache misses count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.MissCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "query_cache_caches_total"),
					"Total query cache caches count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.CacheCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "query_cache_evictions_total"),
					"Total query cache evictions count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.Evictions)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "request_cache_memory_bytes_total"),
					"Total request cache memory bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.RequestCache.MemorySizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "request_cache_hits_total"),
					"Total request cache hits count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.RequestCache.HitCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "request_cache_misses_total"),
					"Total request cache misses count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.RequestCache.MissCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "request_cache_evictions_total"),
					"Total request cache evictions count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.RequestCache.Evictions)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "fielddata_memory_bytes_total"),
					"Total fielddata memory bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Fielddata.MemorySizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "index_stats", "fielddata_evictions_total"),
					"Total fielddata evictions count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Fielddata.Evictions)
				},
				Labels: indexLabels,
			},
		},
		shardMetrics: []*shardMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "shards_docs"),
					"Count of documents on this shard",
					shardLabels.keys(), nil,
				),
				Value: func(data IndexStatsIndexShardsDetailResponse) float64 {
					return float64(data.Docs.Count)
				},
				Labels: shardLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "shards_docs_deleted"),
					"Count of deleted documents on this shard",
					shardLabels.keys(), nil,
				),
				Value: func(data IndexStatsIndexShardsDetailResponse) float64 {
					return float64(data.Docs.Deleted)
				},
				Labels: shardLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "shards_store_size_in_bytes"),
					"Store size of this shard",
					shardLabels.keys(), nil,
				),
				Value: func(data IndexStatsIndexShardsDetailResponse) float64 {
					return float64(data.Store.SizeInBytes)
				},
				Labels: shardLabels,
			},
		},
	}

	// start go routine to fetch clusterinfo updates and save them to lastClusterinfo
	go func() {
		_ = level.Debug(logger).Log("msg", "starting cluster info receive loop")
		for ci := range indices.clusterInfoCh {
			if ci != nil {
				_ = level.Debug(logger).Log("msg", "received cluster info update", "cluster", ci.ClusterName)
				indices.lastClusterInfo = ci
			}
		}
		_ = level.Debug(logger).Log("msg", "exiting cluster info receive loop")
	}()
	return indices
}

// ClusterLabelUpdates returns a pointer to a channel to receive cluster info updates. It implements the
// (not exported) clusterinfo.consumer interface
func (i *Indices) ClusterLabelUpdates() *chan *clusterinfo.Response {
	return &i.clusterInfoCh
}

// String implements the stringer interface. It is part of the clusterinfo.consumer interface
func (i *Indices) String() string {
	return namespace + "indices"
}

// Describe add Indices metrics descriptions
func (i *Indices) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range i.indexMetrics {
		ch <- metric.Desc
	}
	ch <- i.up.Desc()
	ch <- i.totalScrapes.Desc()
	ch <- i.jsonParseFailures.Desc()
}

func (i *Indices) fetchAndDecodeIndexStats() (indexStatsResponse, error) {
	var isr indexStatsResponse

	u := *i.url
	u.Path = path.Join(u.Path, "/_all/_stats")
	if i.shards {
		u.RawQuery = "level=shards"
	}

	res, err := i.client.Get(u.String())
	if err != nil {
		return isr, fmt.Errorf("failed to get index stats from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			_ = level.Warn(i.logger).Log(
				"msg", "failed to close http.Client",
				"err", err,
			)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return isr, fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := ioutil.ReadAll(res.Body)
	if err != nil {
		i.jsonParseFailures.Inc()
		return isr, err
	}

	if err := json.Unmarshal(bts, &isr); err != nil {
		i.jsonParseFailures.Inc()
		return isr, err
	}

	return isr, nil
}

// Collect gets Indices metric values
func (i *Indices) Collect(ch chan<- prometheus.Metric) {
	i.totalScrapes.Inc()
	defer func() {
		ch <- i.up
		ch <- i.totalScrapes
		ch <- i.jsonParseFailures
	}()

	// indices
	indexStatsResp, err := i.fetchAndDecodeIndexStats()
	if err != nil {
		i.up.Set(0)
		_ = level.Warn(i.logger).Log(
			"msg", "failed to fetch and decode index stats",
			"err", err,
		)
		return
	}
	i.up.Set(1)

	// Index stats
	for indexName, indexStats := range indexStatsResp.Indices {
		for _, metric := range i.indexMetrics {
			ch <- prometheus.MustNewConstMetric(
				metric.Desc,
				metric.Type,
				metric.Value(indexStats),
				metric.Labels.values(i.lastClusterInfo, indexName)...,
			)

		}
		if i.shards {
			for _, metric := range i.shardMetrics {
				// gaugeVec := prometheus.NewGaugeVec(metric.Opts, metric.Labels)
				for shardNumber, shards := range indexStats.Shards {
					for _, shard := range shards {
						ch <- prometheus.MustNewConstMetric(
							metric.Desc,
							metric.Type,
							metric.Value(shard),
							metric.Labels.values(i.lastClusterInfo, indexName, shardNumber, shard.Routing.Node, strconv.FormatBool(shard.Routing.Primary))...,
						)
					}
				}
			}
		}
	}
}
