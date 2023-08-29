package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"time"

	hellopbv1 "grpc-go-boilerplate/gen/proto/hello/v1"
	"grpc-go-boilerplate/internal/hello"

	//promgrpc "github.com/grpc-ecosystem/go-grpc-middleware/providers/openmetrics/v2"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/oklog/run"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

func main() {
	flag.Parse()

	// Setup logger
	if *dev {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339Nano})
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
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
	var logger zerolog.Logger

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
			log.Info().Msg("shutting down gracefully, press Ctrl+C again to force")

			// The context is used to inform the server it has 5 seconds to finish
			// the request it is currently handling
			_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Handle cleanup here if any

			log.Info().Msg("Server exiting")
		}
		g.Add(execute, interrupt)
	} // Control-C watcher
	{
		execute := func() error {
			lis, err := net.Listen("tcp", ":"+port)
			if err != nil {
				log.Fatal().Err(err).Msg("failed to create listener")
			}
			//opts := []logging.Option{
			//	logging.WithDecider(func(fullMethodName string, err error) logging.Decision {
			//		// Don't log gRPC calls if it was a call to healthcheck and no error was raised
			//		if fullMethodName == "/grpc.health.v1.Health/Check" {
			//			return logging.NoLogCall
			//		}
			//		// By default, log all calls
			//		return logging.LogStartAndFinishCall
			//	}),
			//}
			logger = zerolog.New(os.Stderr)

			opts := []logging.Option{
				logging.WithLogOnEvents(logging.StartCall, logging.FinishCall),
				// Add any other option (check functions starting with logging.With).
			}
			// Create gRPC server with zerolog, prometheus, and panic recovery middleware
			srv = grpc.NewServer(
				grpc.ChainUnaryInterceptor(
					logging.UnaryServerInterceptor(InterceptorLogger(logger), opts...),
					recovery.UnaryServerInterceptor(),
				),
				grpc.ChainStreamInterceptor(
					logging.StreamServerInterceptor(InterceptorLogger(logger), opts...),
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

			log.Info().Msgf("gRPC server listening on :%s", port)
			if err := srv.Serve(lis); err != nil {
				log.Fatal().Err(err).Msg("failed to start gRPC server")
			}

			return srv.Serve(lis)
		}

		interrupt := func(error) {
			log.Info().Msgf("gRPC server gracefulStop() started")
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
				log.Fatal().Err(err).Msg("failed to connect to register gRPC Gateway Summary service")
			}
			log.Info().Msgf("gRPC Gateway listening on :8081")
			if err := http.ListenAndServe(":8081", mux); err != nil {
				log.Fatal().Err(err).Msg("failed to start gRPC gateway")
			}
			return nil
		}
		interrupt := func(error) {
			log.Info().Msgf("gRPC Gateway gracefulStop() started")
			srv.GracefulStop()
		}
		g.Add(execute, interrupt)
	} // gRPC Gateway

	// Starting the actors
	if err := g.Run(); err != nil {
		log.Fatal().Err(err).Msg("g.Run()_failed")
	}

	log.Info().Msg("Server exiting")
}

func InterceptorLogger(l zerolog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l := l.With().Fields(fields).Logger()

		switch lvl {
		case logging.LevelDebug:
			l.Debug().Msg(msg)
		case logging.LevelInfo:
			l.Info().Msg(msg)
		case logging.LevelWarn:
			l.Warn().Msg(msg)
		case logging.LevelError:
			l.Error().Msg(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}