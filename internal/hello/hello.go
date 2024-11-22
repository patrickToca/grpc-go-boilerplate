package hello

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	goruntime "runtime"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	hellopbv1 "grpc-go-boilerplate/gen/proto/hello/v1"
)

// Infof is an example of a user-defined logging function that wraps slog.
// The log record contains the scd source position of the caller of Infof.
func Infof(logger *slog.Logger, format string, args ...any) {
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	goruntime.Callers(2, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf(format, args...), pcs[0])
	_ = logger.Handler().Handle(context.Background(), r)
}

func replaceDev(groups []string, a slog.Attr) slog.Attr {
	// Remove time.
	if a.Key == slog.TimeKey && len(groups) == 0 {
		return slog.Attr{}
	}
	// Remove the directory from the source's filename.
	if a.Key == slog.SourceKey {
		source := a.Value.Any().(*slog.Source)
		source.File = filepath.Base(source.File)
	}
	return a
}

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
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true, ReplaceAttr: replaceDev}))
	Infof(logger, "%s", "replying to the greeting")
	return h, nil
}