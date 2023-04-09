FROM golang:1.19

COPY . /app
WORKDIR /app

RUN go mod download all
RUN make clean && make generate

RUN go build cmd/grpc-go-boilerplate/main.go

EXPOSE 8080

ENTRYPOINT ["/app/main", "-dev"]