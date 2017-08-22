/*
 * predixy_exporter - scrapes predixy stats and exports for prometheus.
 * Copyright (C) 2017 Joyield, Inc. <joyield.com@gmail.com>
 * All rights reserved.
 */
package main

import (
	"flag"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"predixy_exporter/exporter"
)

func main() {
	var (
		bind = flag.String("bind", ":9617", "Listen address")
		addr = flag.String("addr", "127.0.0.1:7617", "Predixy service address")
		name = flag.String("name", "none", "Redis service name")
	)
	flag.Parse()
	exporter, err := exporter.NewExporter(*addr, *name)
	if err != nil {
		log.Fatal(err)
	}
	prometheus.MustRegister(exporter)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*bind, nil))
}
