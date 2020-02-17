.PHONY: build cluster test

default: build

build:
	go build -o bin/swarm    github.com/athereum/go-athereum/cmd/swarm
	go build -o bin/gath     github.com/athereum/go-athereum/cmd/gath
	go build -o bin/bootnode github.com/athereum/go-athereum/cmd/bootnode

cluster: build
	scripts/boot-cluster.sh

test:
	go test -v github.com/athereum/go-athereum/swarm/...
