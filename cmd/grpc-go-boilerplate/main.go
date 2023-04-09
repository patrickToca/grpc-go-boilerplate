package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/go-chi/chi/v5"
	promgrpc "github.com/grpc-ecosystem/go-grpc-middleware/providers/openmetrics/v2"
	grpczerolog "github.com/grpc-ecosystem/go-grpc-middleware/providers/zerolog/v2"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	hellopbv1 "grpc-go-boilerplate/gen/proto/hello/v1"
	"grpc-go-boilerplate/internal/hello"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const defaultPort = "8080"

var (
	dev = flag.Bool("dev", false, "Enable development mode")
)

func main() {
	flag.Parse()
	var wg sync.WaitGroup
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	// gRPC Server
	wg.Add(1)
	go func() {
		lis, err := net.Listen("tcp", ":"+port)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to create listener")
		}
		opts := []logging.Option{
			logging.WithDecider(func(fullMethodName string, err error) logging.Decision {
				// Don't log gRPC calls if it was a call to healthcheck and no error was raised
				if fullMethodName == "/grpc.health.v1.Health/Check" {
					return logging.NoLogCall
				}
				// By default, log all calls
				return logging.LogStartAndFinishCall
			}),
		}

		// Setup Prometheus metrics
		promGrpcServerMetrics := promgrpc.NewServerMetrics()
		err = promGrpcServerMetrics.Register(prometheus.DefaultRegisterer)
		if err != nil {
			log.Error().Err(err).Msg("failed to register default prometheus registerer")
		}

		// Create gRPC server with zerolog, prometheus, and panic recovery middleware
		srv := grpc.NewServer(
			grpc.ChainUnaryInterceptor(
				logging.UnaryServerInterceptor(grpczerolog.InterceptorLogger(log.Logger), opts...),
				promgrpc.UnaryServerInterceptor(promGrpcServerMetrics),
				recovery.UnaryServerInterceptor(),
			),
			grpc.ChainStreamInterceptor(
				logging.StreamServerInterceptor(grpczerolog.InterceptorLogger(log.Logger), opts...),
				promgrpc.StreamServerInterceptor(promGrpcServerMetrics),
				recovery.StreamServerInterceptor(),
			),
		)

		// Register your services
		greeter := hello.NewGreeter()
		hellopbv1.RegisterHelloServiceServer(srv, greeter)

		// Initialize Prometheus and register reflection
		promGrpcServerMetrics.InitializeMetrics(srv)
		reflection.Register(srv)

		// Health and reflection service
		healthServer := health.NewServer()
		healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		healthServer.SetServingStatus(hellopbv1.HelloService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)
		grpc_health_v1.RegisterHealthServer(srv, healthServer)

		wg.Done()
		log.Info().Msgf("gRPC server listening on :%s", port)
		if err := srv.Serve(lis); err != nil {
			log.Fatal().Err(err).Msg("failed to start gRPC server")
		}
	}()

	// gRPC Gateway
	go func() {
		// Wait for gRPC server to be available before starting gRPC Gateway
		wg.Wait()

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
	}()

	// Prometheus handlers
	go func() {
		// Wait for gRPC server to be available before starting metrics server
		wg.Wait()
		rp := chi.NewRouter()
		rp.Handle("/metrics", promhttp.Handler())
		log.Info().Msg("metrics available on :2112/metrics")
		if err := http.ListenAndServe(":2112", rp); err != nil {
			log.Error().Err(err).Msg("failed to launch prometheus metrics")
		}
	}()

	// Listen for the interrupt signal.
	<-ctx.Done()
	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	log.Info().Msg("shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Handle cleanup here if any

	log.Info().Msg("Server exiting")
}
