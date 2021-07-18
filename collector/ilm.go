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
	"net/http"
	"net/url"
	"path"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

type ilmMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(val int) float64
	Labels func(ilmIndex string, ilmPhase string, ilmAction string, ilmStep string) []string
}

// Index Lifecycle Management information object
type Ilm struct {
	logger log.Logger
	client *http.Client
	url    *url.URL

	up                prometheus.Gauge
	totalScrapes      prometheus.Counter
	jsonParseFailures prometheus.Counter

	ilmMetric ilmMetric
}

// NewIlm defines Index Lifecycle Management Prometheus metrics
func NewIlm(logger log.Logger, client *http.Client, url *url.URL) *Ilm {
	return &Ilm{
		logger: logger,
		client: client,
		url:    url,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "ilm", "up"),
			Help: "Was the last scrape of the ElasticSearch ILM endpoint successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "ilm", "total_scrapes"),
			Help: "Current total ElasticSearch ILM scrapes.",
		}),
		jsonParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "ilm", "json_parse_failures"),
			Help: "Number of errors while parsing JSON.",
		}),
		ilmMetric: ilmMetric{
			Type: prometheus.GaugeValue,
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "ilm", "index_status"),
				"Status of ILM policy for index",
				[]string{"index", "phase", "action", "step"}, nil),
			Value: func(val int) float64 {
				return float64(val)
			},
			Labels: func(ilmIndex string, ilmPhase string, ilmAction string, ilmStep string) []string {
				return []string{ilmIndex, ilmPhase, ilmAction, ilmStep}
			},
		},
	}
}

// Describe adds metrics description
func (i *Ilm) Describe(ch chan<- *prometheus.Desc) {
	ch <- i.ilmMetric.Desc
	ch <- i.up.Desc()
	ch <- i.totalScrapes.Desc()
	ch <- i.jsonParseFailures.Desc()
}

// Bool2int translates boolean variable to its integer alternative
func (i *Ilm) Bool2int(managed bool) int {
	if managed {
		return 1
	} else {
		return 0
	}
}

func (i *Ilm) fetchAndDecodeIlm() (IlmResponse, error) {
	var ir IlmResponse

	u := *i.url
	u.Path = path.Join(u.Path, "/_all/_ilm/explain")

	res, err := i.client.Get(u.String())
	if err != nil {
		return ir, fmt.Errorf("failed to get index stats from %s://%s:%s%s: %s",
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
		return ir, fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	if err := json.NewDecoder(res.Body).Decode(&ir); err != nil {
		i.jsonParseFailures.Inc()
		return ir, err
	}

	return ir, nil
}

// Collect pulls metric values from Elasticsearch
func (i *Ilm) Collect(ch chan<- prometheus.Metric) {
	defer func() {
		ch <- i.up
		ch <- i.totalScrapes
		ch <- i.jsonParseFailures
	}()

	// indices
	ilmResp, err := i.fetchAndDecodeIlm()
	if err != nil {
		i.up.Set(0)
		_ = level.Warn(i.logger).Log(
			"msg", "failed to fetch and decode ILM stats",
			"err", err,
		)
		return
	}
	i.totalScrapes.Inc()
	i.up.Set(1)

	for indexName, indexIlm := range ilmResp.Indices {
		ch <- prometheus.MustNewConstMetric(
			i.ilmMetric.Desc,
			i.ilmMetric.Type,
			i.ilmMetric.Value(i.Bool2int(indexIlm.Managed)),
			i.ilmMetric.Labels(indexName, indexIlm.Phase, indexIlm.Action, indexIlm.Step)...,
		)
	}
}
