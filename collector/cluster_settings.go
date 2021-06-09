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
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/imdario/mergo"
	"github.com/prometheus/client_golang/prometheus"
)

// ClusterSettings information struct
type ClusterSettings struct {
	logger log.Logger
	client *http.Client
	url    *url.URL

	up                     prometheus.Gauge
	totalScrapes           prometheus.Counter
	scrapeDuration         prometheus.Gauge
	jsonParseFailures      prometheus.Counter
	shardAllocationEnabled prometheus.Gauge
	maxShardsPerNode       prometheus.Gauge
}

// NewClusterSettings defines Cluster Settings Prometheus metrics
func NewClusterSettings(logger log.Logger, client *http.Client, url *url.URL) *ClusterSettings {
	return &ClusterSettings{
		logger: logger,
		client: client,
		url:    url,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "clustersettings_stats", "up"),
			Help: "Was the last scrape of the ElasticSearch cluster settings endpoint successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "clustersettings_stats", "total_scrapes"),
			Help: "Current total ElasticSearch cluster settings scrapes.",
		}),
		scrapeDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "clustersettings_stats", "scrape_duration_seconds"),
			Help: "Duration spent in ElasticSearch settings scrape.",
		}),
		shardAllocationEnabled: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "clustersettings_stats", "shard_allocation_enabled"),
			Help: "Current mode of cluster wide shard routing allocation settings.",
		}),
		maxShardsPerNode: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "clustersettings_stats", "max_shards_per_node"),
			Help: "Current maximum number of shards per node setting.",
		}),
		jsonParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "clustersettings_stats", "json_parse_failures"),
			Help: "Number of errors while parsing JSON.",
		}),
	}
}

// Describe add Snapshots metrics descriptions
func (cs *ClusterSettings) Describe(ch chan<- *prometheus.Desc) {
	ch <- cs.up.Desc()
	ch <- cs.scrapeDuration.Desc()
	ch <- cs.totalScrapes.Desc()
	ch <- cs.shardAllocationEnabled.Desc()
	ch <- cs.maxShardsPerNode.Desc()
	ch <- cs.jsonParseFailures.Desc()
}

func (cs *ClusterSettings) getAndParseURL(u *url.URL, data interface{}) error {
	res, err := cs.client.Get(u.String())
	if err != nil {
		return fmt.Errorf("failed to get from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			_ = level.Warn(cs.logger).Log(
				"msg", "failed to close http.Client",
				"err", err,
			)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := ioutil.ReadAll(res.Body)
	if err != nil {
		cs.jsonParseFailures.Inc()
		return err
	}

	if err := json.Unmarshal(bts, data); err != nil {
		cs.jsonParseFailures.Inc()
		return err
	}

	return nil
}

func (cs *ClusterSettings) fetchAndDecodeClusterSettingsStats() (ClusterSettingsResponse, error) {

	u := *cs.url
	u.Path = path.Join(u.Path, "/_cluster/settings")
	q := u.Query()
	q.Set("include_defaults", "true")
	u.RawQuery = q.Encode()
	u.RawPath = q.Encode()
	var csfr ClusterSettingsFullResponse
	var csr ClusterSettingsResponse
	err := cs.getAndParseURL(&u, &csfr)
	if err != nil {
		return csr, err
	}
	err = mergo.Merge(&csr, csfr.Defaults, mergo.WithOverride)
	if err != nil {
		return csr, err
	}
	err = mergo.Merge(&csr, csfr.Persistent, mergo.WithOverride)
	if err != nil {
		return csr, err
	}
	err = mergo.Merge(&csr, csfr.Transient, mergo.WithOverride)

	return csr, err
}

// Collect gets cluster settings  metric values
func (cs *ClusterSettings) Collect(ch chan<- prometheus.Metric) {
	now := time.Now()
	cs.totalScrapes.Inc()
	defer func() {
		_ = level.Debug(cs.logger).Log("msg", "cluster settings scrape took", "seconds", time.Since(now).Seconds())
		cs.scrapeDuration.Set(time.Since(now).Seconds())
		ch <- cs.up
		ch <- cs.totalScrapes
		ch <- cs.scrapeDuration
		ch <- cs.jsonParseFailures
		ch <- cs.shardAllocationEnabled
		ch <- cs.maxShardsPerNode
	}()

	csr, err := cs.fetchAndDecodeClusterSettingsStats()
	if err != nil {
		cs.shardAllocationEnabled.Set(0)
		cs.up.Set(0)
		_ = level.Warn(cs.logger).Log(
			"msg", "failed to fetch and decode cluster settings stats",
			"err", err,
		)
		return
	}
	cs.up.Set(1)

	shardAllocationMap := map[string]int{
		"all":           0,
		"primaries":     1,
		"new_primaries": 2,
		"none":          3,
	}

	cs.shardAllocationEnabled.Set(float64(shardAllocationMap[csr.Cluster.Routing.Allocation.Enabled]))

	maxShardsPerNode, err := strconv.ParseInt(csr.Cluster.MaxShardsPerNode, 10, 64)
	if err == nil {
		cs.maxShardsPerNode.Set(float64(maxShardsPerNode))
	}
}
