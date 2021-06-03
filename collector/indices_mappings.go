package collector

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	defaultIndicesMappingsLabels = []string{"index"}
)

type indicesMappingsMetric struct {
	Type  prometheus.ValueType
	Desc  *prometheus.Desc
	Value func(indexMapping IndexMapping) float64
}

// IndicesMappings information struct
type IndicesMappings struct {
	logger log.Logger
	client *http.Client
	url    *url.URL

	up                              prometheus.Gauge
	totalScrapes, jsonParseFailures prometheus.Counter

	metrics []*indicesMappingsMetric
}

// NewIndicesMappings defines Indices IndexMappings Prometheus metrics
func NewIndicesMappings(logger log.Logger, client *http.Client, url *url.URL) *IndicesMappings {
	subsystem := "indices_mappings_stats"

	return &IndicesMappings{
		logger: logger,
		client: client,
		url:    url,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, subsystem, "up"),
			Help: "Was the last scrape of the ElasticSearch Indices Mappings endpoint successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, subsystem, "total_scrapes"),
			Help: "Current total ElasticSearch Indices Mappings scrapes.",
		}),
		jsonParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, subsystem, "json_parse_failures"),
			Help: "Number of errors while parsing JSON.",
		}),
		metrics: []*indicesMappingsMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "field_count"),
					"Current number fields within cluster.",
					defaultIndicesMappingsLabels, nil,
				),
				Value: func(indexMapping IndexMapping) float64 {
					return countFieldsRecursive(indexMapping.Mappings.Properties, 0)
				},
			},
		},
	}
}

func countFieldsRecursive(properties IndexMappingProperties, fieldCounter float64) float64 {
	// iterate over all properties
	for _, property := range properties {
		if property.Type != nil {
			// property has a type set - counts as a field
			fieldCounter++

			// iterate over all fields of that property
			for _, field := range property.Fields {
				// field has a type set - counts as a field
				if field.Type != nil {
					fieldCounter++
				}
			}
		}

		// count recursively in case the property has more properties
		if property.Properties != nil {
			fieldCounter = +countFieldsRecursive(property.Properties, fieldCounter)
		}
	}

	return fieldCounter
}

// Describe add Snapshots metrics descriptions
func (im *IndicesMappings) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range im.metrics {
		ch <- metric.Desc
	}

	ch <- im.up.Desc()
	ch <- im.totalScrapes.Desc()
	ch <- im.jsonParseFailures.Desc()
}

func (im *IndicesMappings) getAndParseURL(u *url.URL) (*IndicesMappingsResponse, error) {
	res, err := im.client.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		_ = level.Warn(im.logger).Log("msg", "failed to read response body", "err", err)
		return nil, err
	}

	err = res.Body.Close()
	if err != nil {
		_ = level.Warn(im.logger).Log("msg", "failed to close response body", "err", err)
		return nil, err
	}

	var imr IndicesMappingsResponse
	if err := json.Unmarshal(body, &imr); err != nil {
		im.jsonParseFailures.Inc()
		return nil, err
	}

	return &imr, nil
}

func (im *IndicesMappings) fetchAndDecodeIndicesMappings() (*IndicesMappingsResponse, error) {
	u := *im.url
	u.Path = path.Join(u.Path, "/_all/_mappings")
	return im.getAndParseURL(&u)
}

// Collect gets all indices mappings metric values
func (im *IndicesMappings) Collect(ch chan<- prometheus.Metric) {

	im.totalScrapes.Inc()
	defer func() {
		ch <- im.up
		ch <- im.totalScrapes
		ch <- im.jsonParseFailures
	}()

	indicesMappingsResponse, err := im.fetchAndDecodeIndicesMappings()
	if err != nil {
		im.up.Set(0)
		_ = level.Warn(im.logger).Log(
			"msg", "failed to fetch and decode cluster mappings stats",
			"err", err,
		)
		return
	}
	im.up.Set(1)

	for _, metric := range im.metrics {
		for indexName, mappings := range *indicesMappingsResponse {
			ch <- prometheus.MustNewConstMetric(
				metric.Desc,
				metric.Type,
				metric.Value(mappings),
				indexName,
			)
		}
	}
}
