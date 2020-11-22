package main

import (
	"flag"
	"net"
	"net/http"

	"github.com/elizarpif/grpctorrent/api"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	trackerAddr     = "localhost:9000"
	defaultGrpcPort = "9001"
	defaultHttpPort = "8000"
)

func getAddress() (grpcAddr string, httpAddr string) {
	peerPort := flag.String("grpc", defaultGrpcPort, "port for grpc address")

	httpPort := flag.String("http", defaultHttpPort, "port for http address")
	flag.Parse()

	grpcAddr = net.JoinHostPort("localhost", func() string{
		if peerPort == nil{
			return defaultGrpcPort
		}
		return *peerPort
	}())

	httpAddr = net.JoinHostPort("localhost", func() string{
		if peerPort == nil{
			return defaultHttpPort
		}
		return *httpPort
	}())

	return grpcAddr, httpAddr
}

func main() {
	logger := newLogger()
	grpcAddr, httpAddr := getAddress()

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		logger.WithError(err).WithField("address", grpcAddr).Fatal("listen for grpc")
	}

	grpcServer := grpc.NewServer()
	defer grpcServer.GracefulStop()

	ctx := setContext()

	server, err := NewPeer(ctx, trackerAddr, grpcAddr)
	if err != nil {
		logger.WithError(err).Fatal("cannot create peer")
	}

	api.RegisterPeerServer(grpcServer, server)

	mux := runtime.NewServeMux()
	err = api.RegisterPeerHandlerFromEndpoint(ctx, mux, grpcAddr, []grpc.DialOption{grpc.WithInsecure()})
	if err != nil {
		logger.WithError(err).Fatal("cannot register")
	}

	srv := &http.Server{
		Addr:     httpAddr,
		Handler:  mux,
	}

	group := errgroup.Group{}
	group.Go(func() error {
		logger.WithField("grpc_address", grpcAddr).Info("start grpc server")
		return grpcServer.Serve(lis)
	})

	group.Go(func() error {
		logger.WithField("http_address", httpAddr).Info("start http server")
		return srv.ListenAndServe()
	})

	err = group.Wait()
	if err != nil {
		logger.WithError(err).Fatal("group wait")
	}
}
