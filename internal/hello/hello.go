package hello

import (
	"context"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/emptypb"
	hellopbv1 "grpc-go-boilerplate/gen/proto/hello/v1"
)

//Implement the generated HelloServiceServer gRPC interface

type Greeter struct {
	//Embed the unimplemented server and opt-out of forward compatibility
	hellopbv1.UnimplementedHelloServiceServer
}

func NewGreeter() *Greeter {
	return &Greeter{}
}

func (g *Greeter) Hello(ctx context.Context, empty *emptypb.Empty) (*hellopbv1.HelloResponse, error) {
	h := &hellopbv1.HelloResponse{
		Hello: "Hello world!",
	}
	log.Info().Msg("replying to the greeting")
	return h, nil
}