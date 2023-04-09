# gRPC Go Boilerplate 

A minimal repo containing all the boilerplate for getting started with gRPC in Go.

## Features

* Example gRPC service and implementation
* Protobuf & gRPC generation
* Example Middleware
  * Logging
  * Prometheus
  * Panic Recovery
* Health Check
* Example gRPC Gateway integration
* `buf` + plugins integration
* Structured logging
* Prometheus Metrics
* Protobuf and code linting
* Dev mode toggle for development tasks
* Graceful shutdown option
* Makefile with generate, clean, and lint targets
* Dockerfile

## Usage

`go mod download all` to download all dependencies. 

To generate the protobuf and gRPC files run `make generate` or `go generate ./...`

To start the server in dev mode run `go run cmd/grpc-go-boilerplate/main.go -dev`

**Example gRPC Client Call**

```
hello.v1.HelloService@127.0.0.1:8080> call Hello
{
  "hello": "Hello world!"
}

```

**Example HTTP Client Call**

```
curl localhost:8081/v1/hello
{"hello":"Hello world!"}
```

### Dependencies

* [golangci-lint](https://golangci-lint.run/) - Go code linter 

### Credits

* [Go Protobuf Plugin Versioning](https://jbrandhorst.com/post/plugin-versioning/)