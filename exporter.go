package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type VecInfo struct {
	help   string
	labels []string
}

var (
	gaugeMetrics = map[string]string{
		"indices_fielddata_memory_size_bytes":     "Field data cache memory usage in bytes",
		"indices_filter_cache_memory_size_bytes":  "Filter cache memory usage in bytes",
		"indices_query_cache_memory_size_bytes":   "Query cache memory usage in bytes",
		"indices_request_cache_memory_size_bytes": "Request cache memory usage in bytes",
		"indices_docs":                            "Count of documents on this node",
		"indices_docs_deleted":                    "Count of deleted documents on this node",
		"indices_store_size_bytes":                "Current size of stored index data in bytes",
		"indices_segments_memory_bytes":           "Current memory size of segments in bytes",
		"indices_segments_count":                  "Count of index segments on this node",
		"process_cpu_percent":                     "Percent CPU used by process",
		"process_mem_resident_size_bytes":         "Resident memory in use by process in bytes",
		"process_mem_share_size_bytes":            "Shared memory in use by process in bytes",
		"process_mem_virtual_size_bytes":          "Total virtual memory used in bytes",
		"process_open_files_count":                "Open file descriptors",
		"process_max_files_count":                 "Max file descriptors for process",
		"os_mem_used_percent":                     "Percentage of used memory",
		"os_load_average":                         "System load average for the last minute, or -1 if not supported",
	}
	counterMetrics = map[string]string{
		"indices_search_query_total":            "Number of query operations",
		"indices_fielddata_evictions":           "Evictions from field data",
		"indices_filter_cache_evictions":        "Evictions from filter cache",
		"indices_query_cache_evictions":         "Evictions from query cache",
		"indices_request_cache_evictions":       "Evictions from request cache",
		"indices_flush_total":                   "Total flushes",
		"indices_flush_time_ms_total":           "Cumulative flush time in milliseconds",
		"transport_rx_packets_total":            "Count of packets received",
		"transport_rx_size_bytes_total":         "Total number of bytes received",
		"transport_tx_packets_total":            "Count of packets sent",
		"transport_tx_size_bytes_total":         "Total number of bytes sent",
		"indices_store_throttle_time_ms_total":  "Throttle time for index store in milliseconds",
		"indices_indexing_index_total":          "Total index calls",
		"indices_indexing_index_time_ms_total":  "Cumulative index time in milliseconds",
		"indices_merges_total":                  "Total merges",
		"indices_merges_total_docs_total":       "Cumulative docs merged",
		"indices_merges_total_size_bytes_total": "Total merge size in bytes",
		"indices_merges_total_time_ms_total":    "Total time spent merging in milliseconds",
		"indices_refresh_total":                 "Total refreshes",
		"indices_refresh_total_time_ms_total":   "Total time spent refreshing",
	}
	counterVecMetrics = map[string]*VecInfo{
		"jvm_gc_collection_seconds_count": {
			help:   "Count of JVM GC runs",
			labels: []string{"gc"},
		},
		"jvm_gc_collection_seconds_sum": {
			help:   "GC run time in seconds",
			labels: []string{"gc"},
		},
		"process_cpu_time_seconds_sum": {
			help:   "Process CPU time in seconds",
			labels: []string{"type"},
		},
		"thread_pool_completed_count": {
			help:   "Thread Pool operations completed",
			labels: []string{"type"},
		},
		"thread_pool_rejected_count": {
			help:   "Thread Pool operations rejected",
			labels: []string{"type"},
		},
	}

	gaugeVecMetrics = map[string]*VecInfo{
		"breakers_estimated_size_bytes": {
			help:   "Estimated size in bytes of breaker",
			labels: []string{"breaker"},
		},
		"breakers_limit_size_bytes": {
			help:   "Limit size in bytes for breaker",
			labels: []string{"breaker"},
		},
		"filesystem_data_available_bytes": {
			help:   "Available space on block device in bytes",
			labels: []string{"mount", "path"},
		},
		"filesystem_data_free_bytes": {
			help:   "Free space on block device in bytes",
			labels: []string{"mount", "path"},
		},
		"filesystem_data_size_bytes": {
			help:   "Size of block device in bytes",
			labels: []string{"mount", "path"},
		},
		"jvm_memory_committed_bytes": {
			help:   "JVM memory currently committed by area",
			labels: []string{"area"},
		},
		"jvm_memory_used_bytes": {
			help:   "JVM memory currently used by area",
			labels: []string{"area"},
		},
		"jvm_memory_max_bytes": {
			help:   "JVM memory max",
			labels: []string{"area"},
		},
		"thread_pool_active_count": {
			help:   "Thread Pool threads active",
			labels: []string{"type"},
		},
		"thread_pool_largest_count": {
			help:   "Thread Pool largest threads count",
			labels: []string{"type"},
		},
		"thread_pool_queue_count": {
			help:   "Thread Pool operations queued",
			labels: []string{"type"},
		},
		"thread_pool_threads_count": {
			help:   "Thread Pool current threads count",
			labels: []string{"type"},
		},
	}

	clusterHealthActivePrimaryShardsDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "active_primary_shards"),
		"Tthe number of primary shards in your cluster. This is an aggregate total across all indices.",
		[]string{"cluster"}, nil)
	clusterHealthActiveShardsDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "active_shards"),
		"Aggregate total of all shards across all indices, which includes replica shards.",
		[]string{"cluster"}, nil)
	clusterHealthDelayedUnassignedShardsDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "delayed_unassigned_shards"),
		"XXX WHAT DOES THIS MEAN?",
		[]string{"cluster"}, nil)
	clusterHealthInitializingShardsDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "initializing_shards"),
		"Count of shards that are being freshly created.",
		[]string{"cluster"}, nil)
	clusterHealthNumberOfDataNodesDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "number_of_data_nodes"),
		"Number of data nodes in the cluster.",
		[]string{"cluster"}, nil)
	clusterHealthNumberOfInFlightFetchDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "number_of_in_flight_fetch"),
		"The number of ongoing shard info requests.",
		[]string{"cluster"}, nil)
	clusterHealthNumberOfNodesDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "number_of_nodes"),
		"Number of nodes in the cluster.",
		[]string{"cluster"}, nil)
	clusterHealthNumberOfPendingTasksDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "number_of_pending_tasks"),
		"XXX WHAT DOES THIS MEAN?",
		[]string{"cluster"}, nil)
	clusterHealthRelocatingShardsDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "relocating_shards"),
		"The number of shards that are currently moving from one node to another node.",
		[]string{"cluster"}, nil)
	clusterHealthStatusIsGreenDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "status_is_green"),
		"Whether all primary and replica shards are allocated.",
		[]string{"cluster"}, nil)
	clusterHealthStatusIsYellowDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "status_is_yellow"),
		"Some of replica shards is missing.",
		[]string{"cluster"}, nil)
	clusterHealthStatusIsRedDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "status_is_red"),
		"Some of primary shards is missing.",
		[]string{"cluster"}, nil)
	clusterHealthTimedOutDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "timed_out"),
		"XXX WHAT DOES THIS MEAN?",
		[]string{"cluster"}, nil)
	clusterHealthUnassignedShardsDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cluster_health", "unassigned_shards"),
		"The number of shards that exist in the cluster state, but cannot be found in the cluster itself.",
		[]string{"cluster"}, nil)
)

// Exporter collects Elasticsearch stats from the given server and exports
// them using the prometheus metrics package.
type Exporter struct {
	NodesStatsURI    string
	ClusterHealthURI string
	mutex            sync.RWMutex

	up prometheus.Gauge

	gauges      map[string]*prometheus.GaugeVec
	gaugeVecs   map[string]*prometheus.GaugeVec
	counters    map[string]*prometheus.CounterVec
	counterVecs map[string]*prometheus.CounterVec

	allNodes bool
	version  string

	client *http.Client
}

// NewExporter returns an initialized Exporter.
func NewExporter(nodesStatsUri string, clusterHealthUri string, timeout time.Duration, allNodes bool, version string) *Exporter {
	counters := make(map[string]*prometheus.CounterVec, len(counterMetrics))
	counterVecs := make(map[string]*prometheus.CounterVec, len(counterVecMetrics))
	gauges := make(map[string]*prometheus.GaugeVec, len(gaugeMetrics))
	gaugeVecs := make(map[string]*prometheus.GaugeVec, len(gaugeVecMetrics))

	for name, info := range counterVecMetrics {
		counterVecs[name] = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      name,
			Help:      info.help,
		}, append([]string{"cluster", "host", "name"}, info.labels...))
	}

	for name, info := range gaugeVecMetrics {
		gaugeVecs[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      name,
			Help:      info.help,
		}, append([]string{"cluster", "host", "name"}, info.labels...))
	}

	for name, help := range counterMetrics {
		counters[name] = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      name,
			Help:      help,
		}, []string{"cluster", "host", "name"})
	}

	for name, help := range gaugeMetrics {
		gauges[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      name,
			Help:      help,
		}, []string{"cluster", "host", "name"})
	}

	// Init our exporter.
	return &Exporter{
		NodesStatsURI:    nodesStatsUri,
		ClusterHealthURI: clusterHealthUri,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Was the Elasticsearch instance query successful?",
		}),

		counters:    counters,
		counterVecs: counterVecs,
		gauges:      gauges,
		gaugeVecs:   gaugeVecs,

		allNodes: allNodes,
		version:  version,

		client: &http.Client{
			Transport: &http.Transport{
				Dial: func(netw, addr string) (net.Conn, error) {
					c, err := net.DialTimeout(netw, addr, timeout)
					if err != nil {
						return nil, err
					}
					if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
						return nil, err
					}
					return c, nil
				},
			},
		},
	}
}

// Describe describes all the metrics ever exported by the elasticsearch
// exporter. It implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.up.Desc()

	for _, vec := range e.counters {
		vec.Describe(ch)
	}

	for _, vec := range e.gauges {
		vec.Describe(ch)
	}

	for _, vec := range e.counterVecs {
		vec.Describe(ch)
	}

	for _, vec := range e.gaugeVecs {
		vec.Describe(ch)
	}
}

// Collect fetches the stats from configured elasticsearch location and
// delivers them as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	// Reset metrics.
	for _, vec := range e.gaugeVecs {
		vec.Reset()
	}

	for _, vec := range e.counterVecs {
		vec.Reset()
	}

	for _, vec := range e.gauges {
		vec.Reset()
	}

	for _, vec := range e.counters {
		vec.Reset()
	}

	e.up.Set(0)
	defer func() { ch <- e.up }()

	resp, err := e.client.Get(e.NodesStatsURI)
	if err != nil {
		log.Println("Error while querying Elasticsearch for nodes stats:", err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Failed to read nodes stats response body:", err)
		return
	}

	var allStats NodeStatsResponse
	err = json.Unmarshal(body, &allStats)
	if err != nil {
		log.Println("Failed to unmarshal nodes stats JSON into struct:", err)
		return
	}

	// If we aren't polling all nodes, make sure we only got one response.
	if !e.allNodes && len(allStats.Nodes) != 1 {
		log.Println("Unexpected number of nodes returned.")
	}

	for _, stats := range allStats.Nodes {
		// GC Stats
		for collector, gcstats := range stats.JVM.GC.Collectors {
			e.counterVecs["jvm_gc_collection_seconds_count"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, collector).Set(float64(gcstats.CollectionCount))
			e.counterVecs["jvm_gc_collection_seconds_sum"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, collector).Set(float64(gcstats.CollectionTime / 1000))
		}

		// Breaker stats
		for breaker, bstats := range stats.Breakers {
			e.gaugeVecs["breakers_estimated_size_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, breaker).Set(float64(bstats.EstimatedSize))
			e.gaugeVecs["breakers_limit_size_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, breaker).Set(float64(bstats.LimitSize))
		}

		// Thread Pool stats
		for pool, pstats := range stats.ThreadPool {
			e.counterVecs["thread_pool_completed_count"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, pool).Set(float64(pstats.Completed))
			e.counterVecs["thread_pool_rejected_count"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, pool).Set(float64(pstats.Rejected))

			e.gaugeVecs["thread_pool_active_count"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, pool).Set(float64(pstats.Active))
			e.gaugeVecs["thread_pool_threads_count"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, pool).Set(float64(pstats.Threads))
			e.gaugeVecs["thread_pool_largest_count"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, pool).Set(float64(pstats.Largest))
			e.gaugeVecs["thread_pool_queue_count"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, pool).Set(float64(pstats.Queue))
		}

		// JVM Memory Stats
		e.gaugeVecs["jvm_memory_committed_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, "heap").Set(float64(stats.JVM.Mem.HeapCommitted))
		e.gaugeVecs["jvm_memory_used_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, "heap").Set(float64(stats.JVM.Mem.HeapUsed))
		e.gaugeVecs["jvm_memory_max_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, "heap").Set(float64(stats.JVM.Mem.HeapMax))
		e.gaugeVecs["jvm_memory_committed_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, "non-heap").Set(float64(stats.JVM.Mem.NonHeapCommitted))
		e.gaugeVecs["jvm_memory_used_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, "non-heap").Set(float64(stats.JVM.Mem.NonHeapUsed))

		// Indices Stats
		e.counters["indices_search_query_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Search.QueryTotal))

		e.gauges["indices_fielddata_memory_size_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.FieldData.MemorySize))
		e.counters["indices_fielddata_evictions"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.FieldData.Evictions))

		e.gauges["indices_filter_cache_memory_size_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.FilterCache.MemorySize))
		e.counters["indices_filter_cache_evictions"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.FilterCache.Evictions))

		e.gauges["indices_query_cache_memory_size_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.QueryCache.MemorySize))
		e.counters["indices_query_cache_evictions"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.QueryCache.Evictions))

		e.gauges["indices_request_cache_memory_size_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.QueryCache.MemorySize))
		e.counters["indices_request_cache_evictions"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.QueryCache.Evictions))

		e.gauges["indices_docs"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Docs.Count))
		e.gauges["indices_docs_deleted"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Docs.Deleted))

		e.gauges["indices_segments_memory_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Segments.Memory))
		e.gauges["indices_segments_count"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Segments.Count))

		e.gauges["indices_store_size_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Store.Size))
		e.counters["indices_store_throttle_time_ms_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Store.ThrottleTime))

		e.counters["indices_flush_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Flush.Total))
		e.counters["indices_flush_time_ms_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Flush.Time))

		e.counters["indices_indexing_index_time_ms_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Indexing.IndexTime))
		e.counters["indices_indexing_index_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Indexing.IndexTotal))

		e.counters["indices_merges_total_time_ms_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Merges.TotalTime))
		e.counters["indices_merges_total_size_bytes_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Merges.TotalSize))
		e.counters["indices_merges_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Merges.Total))

		e.counters["indices_refresh_total_time_ms_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Refresh.TotalTime))
		e.counters["indices_refresh_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Indices.Refresh.Total))

		// Transport Stats
		e.counters["transport_rx_packets_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Transport.RxCount))
		e.counters["transport_rx_size_bytes_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Transport.RxSize))
		e.counters["transport_tx_packets_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Transport.TxCount))
		e.counters["transport_tx_size_bytes_total"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Transport.TxSize))

		// Process Stats
		e.gauges["process_cpu_percent"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Process.CPU.Percent))
		e.gauges["process_mem_resident_size_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Process.Memory.Resident))
		e.gauges["process_mem_share_size_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Process.Memory.Share))
		e.gauges["process_mem_virtual_size_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Process.Memory.TotalVirtual))
		e.gauges["process_open_files_count"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.Process.OpenFD))

		e.counterVecs["process_cpu_time_seconds_sum"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, "total").Set(float64(stats.Process.CPU.Total / 1000))
		e.counterVecs["process_cpu_time_seconds_sum"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, "sys").Set(float64(stats.Process.CPU.Sys / 1000))
		e.counterVecs["process_cpu_time_seconds_sum"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, "user").Set(float64(stats.Process.CPU.User / 1000))

		// File System Stats
		for _, fsStats := range stats.FS.Data {
			e.gaugeVecs["filesystem_data_available_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, fsStats.Mount, fsStats.Path).Set(float64(fsStats.Available))
			e.gaugeVecs["filesystem_data_free_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, fsStats.Mount, fsStats.Path).Set(float64(fsStats.Free))
			e.gaugeVecs["filesystem_data_size_bytes"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name, fsStats.Mount, fsStats.Path).Set(float64(fsStats.Total))
		}

		// OS Stats
		e.gauges["os_mem_used_percent"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(float64(stats.OS.Mem.UsedPercent))

		var load float64
		if strings.HasPrefix(e.version, "5") {
			load = stats.OS.CPU.LoadAvg["1m"]
		} else if strings.HasPrefix(e.version, "2") {
			json.Unmarshal(stats.OS.LoadAvg, &load)
		} else {
			var loads [3]float64
			json.Unmarshal(stats.OS.LoadAvg, &loads)
			load = loads[0]
		}
		e.gauges["os_load_average"].WithLabelValues(allStats.ClusterName, stats.Host, stats.Name).Set(load)
	}

	// Report metrics.

	for _, vec := range e.counterVecs {
		vec.Collect(ch)
	}

	for _, vec := range e.gaugeVecs {
		vec.Collect(ch)
	}

	for _, vec := range e.counters {
		vec.Collect(ch)
	}

	for _, vec := range e.gauges {
		vec.Collect(ch)
	}

	// Obtain cluster health metrics.
	resp, err = e.client.Get(e.ClusterHealthURI)
	if err != nil {
		log.Println("Error while querying Elasticsearch for cluster health:", err)
		return
	}
	defer resp.Body.Close()

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Failed to read cluster health response body:", err)
		return
	}

	var clusterHealth ClusterHealthResponse
	err = json.Unmarshal(body, &clusterHealth)
	if err != nil {
		log.Println("Failed to unmarshal cluster health JSON into struct:", err)
		return
	}

	ch <- prometheus.MustNewConstMetric(clusterHealthActivePrimaryShardsDesc, prometheus.GaugeValue, float64(clusterHealth.ActivePrimaryShards), clusterHealth.ClusterName)
	ch <- prometheus.MustNewConstMetric(clusterHealthActiveShardsDesc, prometheus.GaugeValue, float64(clusterHealth.ActiveShards), clusterHealth.ClusterName)
	ch <- prometheus.MustNewConstMetric(clusterHealthDelayedUnassignedShardsDesc, prometheus.GaugeValue, float64(clusterHealth.DelayedUnassignedShards), clusterHealth.ClusterName)
	ch <- prometheus.MustNewConstMetric(clusterHealthInitializingShardsDesc, prometheus.GaugeValue, float64(clusterHealth.InitializingShards), clusterHealth.ClusterName)
	ch <- prometheus.MustNewConstMetric(clusterHealthNumberOfDataNodesDesc, prometheus.GaugeValue, float64(clusterHealth.NumberOfDataNodes), clusterHealth.ClusterName)
	ch <- prometheus.MustNewConstMetric(clusterHealthNumberOfInFlightFetchDesc, prometheus.GaugeValue, float64(clusterHealth.NumberOfInFlightFetch), clusterHealth.ClusterName)
	ch <- prometheus.MustNewConstMetric(clusterHealthNumberOfNodesDesc, prometheus.GaugeValue, float64(clusterHealth.NumberOfNodes), clusterHealth.ClusterName)
	ch <- prometheus.MustNewConstMetric(clusterHealthNumberOfPendingTasksDesc, prometheus.GaugeValue, float64(clusterHealth.NumberOfPendingTasks), clusterHealth.ClusterName)
	ch <- prometheus.MustNewConstMetric(clusterHealthRelocatingShardsDesc, prometheus.GaugeValue, float64(clusterHealth.RelocatingShards), clusterHealth.ClusterName)
	ch <- prometheus.MustNewConstMetric(clusterHealthUnassignedShardsDesc, prometheus.GaugeValue, float64(clusterHealth.UnassignedShards), clusterHealth.ClusterName)

	statusIsGreen := 0.0
	statusIsYellow := 0.0
	statusIsRed := 0.0
	if clusterHealth.Status == "green" {
		statusIsGreen = 1.0
	} else if clusterHealth.Status == "yellow" {
		statusIsYellow = 1.0
	} else {
		statusIsRed = 1.0
	}
	ch <- prometheus.MustNewConstMetric(clusterHealthStatusIsGreenDesc, prometheus.GaugeValue, statusIsGreen, clusterHealth.ClusterName)
	ch <- prometheus.MustNewConstMetric(clusterHealthStatusIsYellowDesc, prometheus.GaugeValue, statusIsYellow, clusterHealth.ClusterName)
	ch <- prometheus.MustNewConstMetric(clusterHealthStatusIsRedDesc, prometheus.GaugeValue, statusIsRed, clusterHealth.ClusterName)

	timedOut := 0.0
	if clusterHealth.TimedOut {
		timedOut = 1.0
	}
	ch <- prometheus.MustNewConstMetric(clusterHealthTimedOutDesc, prometheus.GaugeValue, timedOut, clusterHealth.ClusterName)

	// Successfully processed stats.
	e.up.Set(1)
}
