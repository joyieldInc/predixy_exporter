# predixy_exporter
Simple server that scrapes Predixy stats and exports them via HTTP for Prometheus consumption

## Build

It is as simple as:

    $ make

## Running

    $ ./predixy_exporter

With default options, predixy_exporter will listen at 0.0.0.0:9617 and
scrapes predixy(127.0.0.1:7617).
To change default options, see:

    $ ./predixy_exporter --help

## License

Copyright (C) 2017 Joyield, Inc. <joyield.com#gmail.com>

All rights reserved.

License under BSD 3-clause "New" or "Revised" License
