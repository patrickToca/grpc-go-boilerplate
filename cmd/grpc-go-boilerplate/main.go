package main

import (
	"context"
	"flag"
	"fmt"
	slog "log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"syscall"
	"time"

	hellopbv1 "grpc-go-boilerplate/gen/proto/hello/v1"
	"grpc-go-boilerplate/internal/hello"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/credentials/insecure"

	//promgrpc "github.com/grpc-ecosystem/go-grpc-middleware/providers/openmetrics/v2"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/oklog/run"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

const defaultPort = "8080"

var (
	dev = flag.Bool("dev", false, "Enable development mode")
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

// Errorf is an example of a user-defined logging function that wraps slog.
// The log record contains the scd source position of the caller of Errorf.
func Errorf(logger *slog.Logger, format string, args ...any) {
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	goruntime.Callers(2, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slog.LevelError, fmt.Sprintf(format, args...), pcs[0])
	_ = logger.Handler().Handle(context.Background(), r)
}

func main() {
	flag.Parse()

	replacedev := func(groups []string, a slog.Attr) slog.Attr {
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

	replaceprod := func(groups []string, a slog.Attr) slog.Attr {
		// Remove the directory from the source's filename.
		if a.Key == slog.SourceKey {
			source := a.Value.Any().(*slog.Source)
			source.File = filepath.Base(source.File)
		}
		return a
	}

	var gologger *slog.Logger

	// Setup logger
	if *dev {
		gologger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true, ReplaceAttr: replacedev}))
	} else {
		gologger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true, ReplaceAttr: replaceprod}))
	}

	port := defaultPort
	envPort, present := os.LookupEnv("PORT")
	if present {
		port = envPort
	}

	// g define Workers as concurrent actors ([]Actors{execute(), interrupt()) with interrupt handling.
	// Actors are defined as a pair of functions: an execute function, which should run synchronously;
	// and an interrupt function, which, when invoked, should cause the execute function to return.
	var g run.Group
	var srv *grpc.Server

	{
		execute := func() error {
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			// Listen for the interrupt signal.
			<-ctx.Done()

			// Restore default behavior on the interrupt signal and notify user of shutdown.
			stop()
			return nil
		}
		interrupt := func(error) {
			Infof(gologger, "message, %s", "shutting down gracefully, press Ctrl+C again to force")

			// The context is used to inform the server it has 5 seconds to finish
			// the request it is currently handling
			_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Handle cleanup here if any

			Infof(gologger, "message, %s", "Server exiting")
		}
		g.Add(execute, interrupt)
	} // Control-C watcher
	{
		execute := func() error {
			lis, err := net.Listen("tcp", ":"+port)
			if err != nil {
				Errorf(gologger, "message, %s", "failed to create listener")
				os.Exit(1)
			}

			opts := []logging.Option{
				logging.WithLogOnEvents(logging.StartCall, logging.FinishCall),
				// Add any other option (check functions starting with logging.With).
			}
			// Create gRPC server with slog, prometheus, and panic recovery middleware
			srv = grpc.NewServer(
				grpc.ChainUnaryInterceptor(
					logging.UnaryServerInterceptor(InterceptorLogger(gologger), opts...),
					recovery.UnaryServerInterceptor(),
				),
				grpc.ChainStreamInterceptor(
					logging.StreamServerInterceptor(InterceptorLogger(gologger), opts...),
					recovery.StreamServerInterceptor(),
				),
			)

			// Register your services
			greeter := hello.NewGreeter()
			hellopbv1.RegisterHelloServiceServer(srv, greeter)

			reflection.Register(srv)

			// Health and reflection service
			healthServer := health.NewServer()
			healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
			healthServer.SetServingStatus(hellopbv1.HelloService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)
			grpc_health_v1.RegisterHealthServer(srv, healthServer)

			//log.Info().Msgf("gRPC server listening on :%s", port)
			Infof(gologger, "gRPC server listening on :%s", port)
			if err := srv.Serve(lis); err != nil {
				//log.Fatal().Err(err).Msg("failed to start gRPC server")
				Errorf(gologger, "message, %s", "failed to start gRPC server")
				os.Exit(1)
			}

			return srv.Serve(lis)
		}

		interrupt := func(error) {
			//log.Info().Msgf("gRPC server gracefulStop() started")
			Infof(gologger, "message :%s", "gRPC server gracefulStop() started")
			srv.GracefulStop()
		}

		g.Add(execute, interrupt)
	} // gRPC Server
	{
		execute := func() error {
			mux := runtime.NewServeMux()
			opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
			grpcEndpoint := fmt.Sprintf("localhost:%s", port)
			err := hellopbv1.RegisterHelloServiceHandlerFromEndpoint(context.Background(), mux, grpcEndpoint, opts)
			if err != nil {
				//log.Fatal().Err(err).Msg("failed to connect to register gRPC Gateway Summary service")
				Errorf(gologger, "message, %s", "failed to connect to register gRPC Gateway Summary service")
				os.Exit(1)
			}
			//log.Info().Msgf("gRPC Gateway listening on :8081")
			Infof(gologger, "message :%s", "gRPC Gateway listening on :8081")
			if err := http.ListenAndServe(":8081", mux); err != nil {
				//log.Fatal().Err(err).Msg("failed to start gRPC gateway")
				Errorf(gologger, "message, %s", "failed to start gRPC gateway")
				os.Exit(1)
			}
			return nil
		}
		interrupt := func(error) {
			//log.Info().Msgf("gRPC Gateway gracefulStop() started")
			Infof(gologger, "message :%s", "gRPC Gateway gracefulStop() started")
			srv.GracefulStop()
		}
		g.Add(execute, interrupt)
	} // gRPC Gateway

	// Starting the actors
	if err := g.Run(); err != nil {
		//log.Fatal().Err(err).Msg("g.Run()_failed")
		Errorf(gologger, "message, %s", "g.Run()_failed")
		os.Exit(1)
	}

	//log.Info().Msg("Server exiting")
	Infof(gologger, "message :%s", "Server exiting")
}

func InterceptorLogger(l *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		//l := l.With().Fields(fields).Logger()

		switch lvl {
		case logging.LevelDebug:
			//l.Debug().Msg(msg)
		case logging.LevelInfo:
			//l.Info().Msg(msg)
			Infof(l, "message, %s", msg)
		case logging.LevelWarn:
			//l.Warn().Msg(msg)
		case logging.LevelError:
			//l.Error().Msg(msg)
			Errorf(l, "message, %s", msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}