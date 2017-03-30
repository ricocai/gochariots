#!/bin/sh

nohup gochariots-app 8080 1 0 > $0.log &
nohup gochariots-controller 8081 1 0 >> $0.log &

sleep 1

nohup gochariots-batcher 9000 1 0 >> $0.log &
nohup gochariots-batcher 9001 1 0 >> $0.log &
nohup gochariots-batcher 9002 1 0 >> $0.log &
curl -XPOST localhost:8080/batcher?host=localhost:9000
curl -XPOST localhost:8080/batcher?host=localhost:9001
curl -XPOST localhost:8080/batcher?host=localhost:9002
curl -XPOST localhost:8081/batcher?host=localhost:9000
curl -XPOST localhost:8081/batcher?host=localhost:9001
curl -XPOST localhost:8081/batcher?host=localhost:9002

nohup gochariots-filter 9010 1 0 >> $0.log &
curl -XPOST localhost:8081/filter?host=localhost:9010
curl -XGET localhost:8081/filter

nohup gochariots-queue 9020 1 0 true >> $0.log &
nohup gochariots-queue 9021 1 0 false >> $0.log &
nohup gochariots-queue 9022 1 0 false >> $0.log &

curl -XPOST localhost:8081/queue?host=localhost:9020
curl -XPOST localhost:8081/queue?host=localhost:9021
curl -XPOST localhost:8081/queue?host=localhost:9022

nohup gochariots-maintainer 9030 1 0 >> $0.log &
nohup gochariots-maintainer 9031 1 0 >> $0.log &
nohup gochariots-maintainer 9032 1 0 >> $0.log &

curl -XPOST localhost:8081/maintainer?host=localhost:9030
curl -XPOST localhost:8081/maintainer?host=localhost:9031
curl -XPOST localhost:8081/maintainer?host=localhost:9032
curl -XGET localhost:8081/maintainer

tail -f $0.log
