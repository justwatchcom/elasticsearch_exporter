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

var (
	defaultNodeHTTPLabelValues = func(cluster string, node NodeHTTPStatsNode) []string {
		roles := getRoles(node)
		return []string{
			cluster,
			node.Host,
			node.Name,
			fmt.Sprintf("%t", roles["master"]),
			fmt.Sprintf("%t", roles["data"]),
			fmt.Sprintf("%t", roles["ingest"]),
			fmt.Sprintf("%t", roles["client"]),
		}
	}
)

type nodeHTTPMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(node NodeHTTPStatsNode) float64
	Labels func(cluster string, node NodeHTTPStatsNode) []string
}

// NodesHTTP information struct
type NodesHTTP struct {
	logger            log.Logger
	client            *http.Client
	url               *url.URL
	up                prometheus.Gauge
	totalScrapes      prometheus.Counter
	jsonParseFailures prometheus.Counter
	metrics           []*nodeHTTPMetric
}

// NewNodesHTTP defines Nodes HTTP Prometheus metrics
func NewNodesHTTP(logger log.Logger, client *http.Client, url *url.URL) *NodesHTTP {
	return &NodesHTTP{
		logger: logger,
		client: client,
		url:    url,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "node_stats_http", "up"),
			Help: "Was the last scrape of the ElasticSearch nodes endpoint successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "node_stats_http", "total_scrapes"),
			Help: "Current total ElasticSearch node scrapes.",
		}),
		jsonParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "node_stats_http", "json_parse_failures"),
			Help: "Number of errors while parsing JSON.",
		}),

		metrics: []*nodeHTTPMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "node_stats_http", "open"),
					"Current number of open connections",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeHTTPStatsNode) float64 {
					return float64(node.HTTP.CurrentOpen)
				},
				Labels: defaultNodeHTTPLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "node_stats_http", "opened_total"),
					"Total number of http connection opened",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeHTTPStatsNode) float64 {
					return float64(node.HTTP.TotalOpened)
				},
				Labels: defaultNodeHTTPLabelValues,
			},
		},
	}
}

// Describe add metrics descriptions
func (c *NodesHTTP) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range c.metrics {
		ch <- metric.Desc
	}
	ch <- c.up.Desc()
	ch <- c.totalScrapes.Desc()
	ch <- c.jsonParseFailures.Desc()
}

func (c *NodesHTTP) fetchAndDecodeNodeStats() (nodeHTTPStatsResponse, error) {
	var nsr nodeHTTPStatsResponse

	u := *c.url
	u.Path = path.Join(u.Path, "/_nodes/stats/http")

	res, err := c.client.Get(u.String())
	if err != nil {
		return nsr, fmt.Errorf("failed to get cluster health from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			_ = level.Warn(c.logger).Log(
				"msg", "failed to close http.Client",
				"err", err,
			)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return nsr, fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	if err := json.NewDecoder(res.Body).Decode(&nsr); err != nil {
		c.jsonParseFailures.Inc()
		return nsr, err
	}
	return nsr, nil
}

// Collect gets nodes metric values
func (c *NodesHTTP) Collect(ch chan<- prometheus.Metric) {
	c.totalScrapes.Inc()
	defer func() {
		ch <- c.up
		ch <- c.totalScrapes
		ch <- c.jsonParseFailures
	}()

	nodeStatsResp, err := c.fetchAndDecodeNodeStats()
	if err != nil {
		c.up.Set(0)
		_ = level.Warn(c.logger).Log(
			"msg", "failed to fetch and decode node stats",
			"err", err,
		)
		return
	}
	c.up.Set(1)

	for _, node := range nodeStatsResp.Nodes {
		for _, metric := range c.metrics {
			ch <- prometheus.MustNewConstMetric(
				metric.Desc,
				metric.Type,
				metric.Value(node),
				metric.Labels(nodeStatsResp.ClusterName, node)...,
			)
		}
	}
}
