/*
 * predixy_exporter - scrapes predixy stats and exports for prometheus.
 * Copyright (C) 2017 Joyield, Inc. <joyield.com@gmail.com>
 * All rights reserved.
 */
package exporter

import (
	"github.com/garyburd/redigo/redis"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"strconv"
	"strings"
	"sync"
)

const (
	namespace    = "predixy"
	globalScope  = 0
	serverScope  = 1
	latencyScope = 2
)

var (
	globalGauges = [][]string{
		{"UsedMemory", "used_memory", "Current alloc memory"},
		{"UsedCpu", "used_cpu", "Used cpu"},
		{"TotalRequests", "total_requests", "Total requests"},
		{"TotalResponses", "total_responses", "Total responses"},
		{"TotalRecvClientBytes", "total_recv_client_bytes", "Total recv client bytes"},
		{"TotalSendClientBytes", "total_send_client_bytes", "Total send client bytes"},
	}
	clusterGauges = [][]string{
		{"UsedMemory", "used_memory", "Current alloc memory"},
		{"MaxMemory", "max_memory", "Current max memory setting"},
		{"MaxRSS", "max_rss", "Max rss used"},
		{"UsedCpuSys", "used_cpu_sys", "Used cpu sys"},
		{"UsedCpuUser", "used_cpu_user", "Used cpu user"},
		{"UsedCpu", "used_cpu", "Used cpu"},
		{"Accept", "accept", "Acceptted connections"},
		{"ClientConnections", "connections", "Current client connections"},
		{"TotalRequests", "total_requests", "Total requests"},
		{"TotalResponses", "total_respones", "Total responses"},
		{"TotalRecvClientBytes", "total_recv_client_bytes", "Total recv client bytes"},
		{"TotalSendServerBytes", "total_send_server_bytes", "Total send server bytes"},
		{"TotalRecvServerBytes", "total_recv_server_bytes", "Total recv server bytes"},
		{"TotalSendClientBytes", "total_send_client_bytes", "Total send client bytes"},
	}
	servGauges = [][]string{
		{"Connections", "connections", "Server connections"},
		{"Connect", "connect", "Server connect count"},
		{"Requests", "requests", "Server requests"},
		{"Responses", "responses", "Server responses"},
		{"SendBytes", "send_bytes", "Server send bytes"},
		{"RecvBytes", "recv_bytes", "Server recv bytes"},
	}
	servLabels        = []string{"server"}
	latencyLabels     = []string{"latency"}
	servLatencyLabels = []string{"server", "latency"}
	latencyBuckets    = append(prometheus.LinearBuckets(20, 20, 100), prometheus.ExponentialBuckets(2048., 1.06437, 100)...)
)

type Exporter struct {
	mutex           sync.RWMutex
	addr            string
	name            string
	globalGauges    map[string]prometheus.Gauge
	clusterGauges   map[string]prometheus.Gauge
	servGauges      map[string]*prometheus.GaugeVec
	latencyDesc     *prometheus.Desc
	servLatencyDesc *prometheus.Desc
	latencyMetrics  []prometheus.Metric
}

func NewExporter(addr, name string) (*Exporter, error) {
	e := &Exporter{
		addr:          addr,
		name:          name,
		globalGauges:  map[string]prometheus.Gauge{},
		clusterGauges: map[string]prometheus.Gauge{},
		servGauges:    map[string]*prometheus.GaugeVec{},
		latencyDesc: prometheus.NewDesc(
			namespace+"_"+name+"_latency",
			"Latency",
			latencyLabels,
			prometheus.Labels{"addr": addr}),
		servLatencyDesc: prometheus.NewDesc(
			namespace+"_"+name+"_server_latency",
			"Server latency",
			servLatencyLabels,
			prometheus.Labels{"addr": addr}),
		latencyMetrics: []prometheus.Metric{},
	}
	for _, m := range globalGauges {
		e.globalGauges[m[0]] = prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   namespace,
			Name:        m[1],
			Help:        m[2],
			ConstLabels: prometheus.Labels{"cluster": name, "addr": addr},
		})
	}
	for _, m := range clusterGauges {
		e.clusterGauges[m[0]] = prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   namespace,
			Subsystem:   name,
			Name:        m[1],
			Help:        m[2],
			ConstLabels: prometheus.Labels{"addr": addr},
		})
	}
	for _, m := range servGauges {
		e.servGauges[m[0]] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   namespace,
			Subsystem:   name + "_server",
			Name:        m[1],
			Help:        m[2],
			ConstLabels: prometheus.Labels{"addr": addr},
		}, servLabels)
	}
	return e, nil
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, g := range e.globalGauges {
		ch <- g.Desc()
	}
	for _, g := range e.clusterGauges {
		ch <- g.Desc()
	}
	for _, g := range e.servGauges {
		g.Describe(ch)
	}
	ch <- e.latencyDesc
	ch <- e.servLatencyDesc
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.latencyMetrics = e.latencyMetrics[:0]
	e.resetMetrics()
	e.scrape()
	for _, g := range e.globalGauges {
		ch <- g
	}
	for _, g := range e.clusterGauges {
		ch <- g
	}
	for _, g := range e.servGauges {
		g.Collect(ch)
	}
	for _, m := range e.latencyMetrics {
		ch <- m
	}
}

func (e *Exporter) resetMetrics() {
	for _, g := range e.servGauges {
		g.Reset()
	}
}

type Bucket struct {
	bound uint64
	value uint64
	count uint64
}

func (e *Exporter) scrape() {
	c, err := redis.Dial("tcp", e.addr)
	if err != nil {
		log.Printf("dial redis %s err:%q\n", e.addr, err)
		return
	}
	defer c.Close()
	r, err := redis.String(c.Do("INFO"))
	if err != nil {
		log.Printf("redis %s do INFO err:%q\n", e.addr, err)
		return
	}
	cpu := 0.
	scope := globalScope
	server := ""
	servs := []string{}
	latency := ""
	lines := strings.Split(r, "\n")
	for len(lines) > 0 {
		line := lines[0]
		lines = lines[1:]
		switch scope {
		case globalScope:
			if strings.HasPrefix(line, "# Servers") {
				scope = serverScope
				continue
			} else if strings.HasPrefix(line, "# LatencyMonitor") {
				scope = latencyScope
				continue
			}
			s := strings.Split(line, ":")
			if len(s) == 2 {
				g0, ok0 := e.globalGauges[s[0]]
				g1, ok1 := e.clusterGauges[s[0]]
				if ok0 || ok1 {
					v, err := strconv.ParseFloat(s[1], 64)
					if err == nil {
						if ok0 {
							g0.Set(v)
						}
						if ok1 {
							g1.Set(v)
						}
					}
					if s[0] == "UsedCpuSys" || s[0] == "UsedCpuUser" {
						cpu += v
					}
				}
			}
		case serverScope:
			s := strings.Split(line, ":")
			if len(s) >= 2 {
				g, ok := e.servGauges[s[0]]
				if ok {
					v, err := strconv.ParseFloat(s[1], 64)
					if err == nil {
						g.WithLabelValues(server).Set(v)
					}
				} else if s[0] == "Server" {
					server = s[1] + ":" + s[2]
					servs = append(servs, server)
				}
			} else if strings.HasPrefix(line, "# LatencyMonitor") {
				scope = latencyScope
			}
		case latencyScope:
			if strings.HasPrefix(line, "LatencyMonitorName") {
				latency = strings.Split(line, ":")[1]
				buckets, last, lineNum, ok := e.parseBuckets(lines)
				if ok {
					e.addLatencyMetrics(buckets, last, false, latency)
				}
				lines = lines[lineNum:]
			}
		default:
		}

	}
	if g, ok := e.globalGauges["UsedCpu"]; ok {
		g.Set(cpu)
	}
	if g, ok := e.clusterGauges["UsedCpu"]; ok {
		g.Set(cpu)
	}
	if len(latency) == 0 || len(servs) == 0 {
		return
	}
	for _, server := range servs {
		r, err = redis.String(c.Do("INFO", "ServerLatency", server))
		if err != nil {
			log.Printf("redis %s INFO ServerLatency %s error:%q", e.addr, server, err)
			continue
		}
		latency = ""
		lines = strings.Split(r, "\n")
		for len(lines) > 0 {
			line := lines[0]
			lines = lines[1:]
			if strings.HasPrefix(line, "ServerLatencyMonitorName") {
				latency = strings.Split(line, " ")[1]
				buckets, last, lineNum, ok := e.parseBuckets(lines)
				if ok {
					e.addLatencyMetrics(buckets, last, true, server, latency)
				}
				lines = lines[lineNum:]
			}
		}
	}
}

func (e *Exporter) parseBuckets(lines []string) (buckets []Bucket, last Bucket, lineNum int, ok bool) {
	ok = false
	buckets = make([]Bucket, 0)
	for lineNum = 0; lineNum < len(lines); {
		line := lines[lineNum]
		lineNum++
		s := strings.Fields(line)
		if len(s) < 4 {
			return
		}
		bound, err := strconv.ParseUint(s[1], 10, 64)
		if err != nil {
			return
		}
		elapsed, err := strconv.ParseUint(s[2], 10, 64)
		if err != nil {
			return
		}
		count, err := strconv.ParseUint(s[3], 10, 64)
		if err != nil {
			return
		}
		if s[0] == "<=" {
			buckets = append(buckets, Bucket{bound, elapsed, count})
		} else if s[0] == ">" {
			last.bound, last.value, last.count = bound, elapsed, count
		} else if s[0] == "T" {
			ok = true
			return
		} else {
			return
		}
	}
	return
}

func (e *Exporter) addLatencyMetrics(buckets []Bucket, last Bucket, server bool, labels ...string) {
	vc := make(map[float64]uint64)
	var b float64
	i := 0
	j := 0
	value := float64(0)
	count := uint64(0)
	for i, b = range latencyBuckets {
		for j < len(buckets) {
			if float64(buckets[j].bound) <= b {
				value += float64(buckets[j].value)
				count += buckets[j].count
				j++
			} else {
				break
			}
		}
		vc[latencyBuckets[i]] = count
		if j >= len(buckets) {
			break
		}
	}
	if i < len(latencyBuckets) {
		if last.count > 0 {
			avg := float64(last.value) / float64(last.count)
			for ; i < len(latencyBuckets); i++ {
				if latencyBuckets[i] < avg {
					vc[latencyBuckets[i]] = count
				} else {
					count += last.count
					break
				}
			}
		}
		for ; i < len(latencyBuckets); i++ {
			vc[latencyBuckets[i]] = count
		}
	} else {
		for ; j < len(buckets); j++ {
			value += float64(buckets[j].value)
			count += buckets[j].count
		}
		count += last.count
	}
	value += float64(last.value)
	var h prometheus.Metric
	if server {
		h = prometheus.MustNewConstHistogram(e.servLatencyDesc, count, value, vc, labels...)
	} else {
		h = prometheus.MustNewConstHistogram(e.latencyDesc, count, value, vc, labels...)
	}
	e.latencyMetrics = append(e.latencyMetrics, h)
}
