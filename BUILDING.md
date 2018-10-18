# Building

## Download this repo locally

```
$ go get https://github.com/HotelsDotCom/node-reaper
```

## Build the binary and container
```
$ make clean build
```

You will need the following in order to build node reaper in docker:

* Git
* Make
* Docker

## Build the binary locally without docker or plugins
```
$ make bin
```

You will need the following in order to build node reaper locally:

* Git
* Make
* Go >=1.10

## Verify the local container is known to your Docker daemon

```
$ docker images | grep -i "node-reaper "

node-reaper                                                         358d999                 d83488882e8b        46 hours ago        61.4MB
```

> Versions are defined by the current checked out git tag, failing that the current checked out git short hash is used instead.

## Running the container locally
```
$ make run
```

It assumes:
 * You have a valid kubeconfig at $KUBECONFIG.
 * You have a valid node reaper configuration file at $PWD/node-reaper.yaml.
 * If using plugins, you have the compiled plugins (.so) at $PWD/bin/plugins.
 * If using the aws node provider plugin, you have the required [aws environment variables](https://docs.aws.amazon.com/cli/latest/userguide/cli-environment.html) exported in the current shell.
 * You have built the current checked out version of node-reaper with `make build`.

## Running the binary locally
```
$ go run cmd/nodereaper/main.go
```

> Plugins on Mac OS are not currently supported. If you want to use plugins and are running on Mac OS run the container instead.
