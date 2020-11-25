package main

import (
	"net"
	"net/http"

	"github.com/elizarpif/grpctorrent/api"
	"github.com/elizarpif/logger"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	grpcAddress = "localhost:9000"
	httpAddr    = "localhost:8000"
)

func main() {
	log := logger.NewLogger()

	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		log.WithError(err).WithField("address", grpcAddress).Fatal("listen for grpc")
	}

	grpcServer := grpc.NewServer()
	defer grpcServer.GracefulStop()

	server := NewServer()
	api.RegisterTrackerServer(grpcServer, server)

	ctx := logger.SetContext(log)

	mux := runtime.NewServeMux()
	err = api.RegisterTrackerHandlerFromEndpoint(ctx, mux, grpcAddress, []grpc.DialOption{grpc.WithInsecure()})
	if err != nil {
		log.WithError(err).Fatal("cannot register")
	}

	srv := &http.Server{
		Addr:    httpAddr,
		Handler: mux,
	}

	group := errgroup.Group{}
	group.Go(func() error {
		log.WithField("address", grpcAddress).Info("start grpc server")
		return grpcServer.Serve(lis)
	})

	group.Go(func() error {
		log.WithField("address", httpAddr).Info("start http server")
		return srv.ListenAndServe()
	})

	err = group.Wait()
	if err != nil {
		log.WithError(err).Fatal("group wait")
	}
}
