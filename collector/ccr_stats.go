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
	"github.com/go-kit/kit/log/level"
	"io/ioutil"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"net/url"
	"path"
	"strconv"
)

// CCRStats information struct
type CCRStats struct {
	logger log.Logger
	client *http.Client
	url    *url.URL

	up                              prometheus.Gauge
	totalScrapes, jsonParseFailures prometheus.Counter
	ccrMetrics                      []*IndexCCRStatsShardMetric
}

type IndexCCRStatsShardMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(shardCCRStats IndexCCRStatsShardResponse) float64
	Labels func(indexCCRStatsShard IndexCCRStatsShardResponse) []string
}

var (
	defaultCCRStatsLabels = []string{"follower_index", "leader_index", "remote_cluster", "shard_id"}
	defaultCCRLabelValues = func(indexCCRStatsShard IndexCCRStatsShardResponse) []string {
		return []string{indexCCRStatsShard.FollowerIndex, indexCCRStatsShard.LeaderIndex, indexCCRStatsShard.RemoteCluster, strconv.Itoa(int(indexCCRStatsShard.ShardID))}
	}
)

// NewCCRStats defines CCRStats Prometheus metrics
func NewCCRStats(logger log.Logger, client *http.Client, url *url.URL) *CCRStats {
	return &CCRStats{
		logger: logger,
		client: client,
		url:    url,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "ccr_stats", "up"),
			Help: "Was the last scrape of the ElasticSearch ccr stats endpoint successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "ccr_stats", "total_scrapes"),
			Help: "Current total ElasticSearch ccr stats scrapes.",
		}),
		jsonParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "ccr_stats", "json_parse_failures"),
			Help: "Number of errors while parsing JSON.",
		}),
		ccrMetrics: []*IndexCCRStatsShardMetric{
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "leader_global_checkpoint"),
					"The current global checkpoint on the follower",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.LeaderGlobalCheckpoint)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "leader_max_seq_no"),
					"The current maximum sequence number on the leader known to the follower task",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.LeaderMaxSeqNo)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "follower_global_checkpoint"),
					"The current global checkpoint on the follower",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.FollowerGlobalCheckpoint)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "follower_max_seq_no"),
					"The current maximum sequence number on the follower",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.FollowerMaxSeqNo)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "last_requested_seq_no"),
					"The starting sequence number of the last batch of operations requested from the leader",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.LastRequestedSeqNo)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "outstanding_read_requests"),
					"The number of active read requests from the follower",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.OutstandingReadRequests)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "outstanding_write_requests"),
					"The number of active bulk write requests on the follower",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.OutstandingWriteRequests)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "write_buffer_operation_count"),
					"The number of write operations queued on the follower",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.WriteBufferOperationCount)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "write_buffer_size_in_bytes"),
					"The total number of bytes of operations currently queued for writing",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.WriteBufferSizeInBytes)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "follower_mapping_version"),
					"The mapping version the follower is synced up to",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.FollowerMappingVersion)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "follower_settings_version"),
					"The index settings version the follower is synced up to",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.FollowerSettingsVersion)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "follower_aliases_version"),
					"The index aliases version the follower is synced up to",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.FollowerAliasesVersion)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "total_read_time_millis"),
					"The total time reads were outstanding, measured from the time a read was sent to the leader to the time a reply was returned to the follower",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.TotalReadTimeMillis)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "total_read_remote_exec_time_millis"),
					"The total time reads spent executing on the remote cluster",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.TotalReadRemoteExecTimeMillis)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "successful_read_requests"),
					"The number of successful fetches",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.SuccessfulReadRequests)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "failed_read_requests"),
					"The number of failed reads",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.FailedReadRequests)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "operations_read"),
					"The total number of operations read from the leader",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.OperationsRead)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "bytes_read"),
					"The total of transferred bytes read from the leader. This is only an estimate and does not account for compression if enabled",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.BytesRead)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "total_write_time_millis"),
					"The total time spent writing on the follower",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.TotalWriteTimeMillis)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "successful_write_requests"),
					"The number of bulk write requests executed on the follower",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.SuccessfulWriteRequests)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "failed_write_requests"),
					"The number of failed bulk write requests executed on the follower",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.FailedWriteRequests)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "operations_written"),
					"The number of operations written on the follower",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.OperationsWritten)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "time_since_last_read_millis"),
					"The number of milliseconds since a read request was sent to the leader",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.TimeSinceListReadMillis)
				},
				Labels: defaultCCRLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "ccr_stats", "replication_lag"),
					"Represents value of how much follower lagging the leader. Based on difference between leader_global_checkpoint and follower_global_checkpoint",
					defaultCCRStatsLabels, nil,
				),
				Value: func(indexCCRStatsShard IndexCCRStatsShardResponse) float64 {
					return float64(indexCCRStatsShard.LeaderGlobalCheckpoint - indexCCRStatsShard.FollowerGlobalCheckpoint)
				},
				Labels: defaultCCRLabelValues,
			},
		},
	}
}

func (s *CCRStats) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range s.ccrMetrics {
		ch <- metric.Desc
	}
	ch <- s.up.Desc()
	ch <- s.totalScrapes.Desc()
	ch <- s.jsonParseFailures.Desc()
}

func (s *CCRStats) fetchCCRStatsResponse() (CCRStatsResponse, error) {
	var ccrResponse CCRStatsResponse

	u := *s.url
	u.Path = path.Join(u.Path, "/_all/_ccr/stats")

	res, err := s.client.Get(u.String())
	if err != nil {
		return ccrResponse, fmt.Errorf("failed to get ccr statistics from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			_ = level.Warn(s.logger).Log(
				"msg", "failed to close http.Client",
				"err", err,
			)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return ccrResponse, fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := ioutil.ReadAll(res.Body)
	if err != nil {
		s.jsonParseFailures.Inc()
		return ccrResponse, err
	}

	if err := json.Unmarshal(bts, &ccrResponse); err != nil {
		s.jsonParseFailures.Inc()
		return ccrResponse, err
	}
	return ccrResponse, err
}

//Collect gets CCRStats metric values
func (s *CCRStats) Collect(ch chan<- prometheus.Metric) {

	s.totalScrapes.Inc()
	defer func() {
		ch <- s.up
		ch <- s.totalScrapes
		ch <- s.jsonParseFailures
	}()

	indexCCRStatsResponse, err := s.fetchCCRStatsResponse()
	if err != nil {
		s.up.Set(0)
		_ = level.Warn(s.logger).Log(
			"msg", "failed to fetch and decode CCRStats stats",
			"err", err,
		)
		return
	}
	s.up.Set(1)

	for _, indexCCRStatsResponse := range indexCCRStatsResponse.IndexCCRStats {
		for _, indexCCRStatsShard := range indexCCRStatsResponse.IndexCCRStatsShards {
			for _, metric := range s.ccrMetrics {
				ch <- prometheus.MustNewConstMetric(
					metric.Desc,
					metric.Type,
					metric.Value(indexCCRStatsShard),
					metric.Labels(indexCCRStatsShard)...,
				)
			}
		}
	}
}
