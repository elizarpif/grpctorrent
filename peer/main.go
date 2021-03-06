package main

import (
	"flag"
	"net"
	"net/http"

	"github.com/elizarpif/grpctorrent/api"
	"github.com/elizarpif/logger"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	trackerAddr     = "localhost:9000"
	defaultGrpcPort = "9001"
	defaultHttpPort = "8000"
)

func getAddress() (grpcAddr, httpAddr string) {
	peerPort := flag.String("grpc", defaultGrpcPort, "port for grpc address")

	httpPort := flag.String("http", defaultHttpPort, "port for http address")
	flag.Parse()

	grpcAddr = net.JoinHostPort("localhost", func() string {
		if peerPort == nil {
			return defaultGrpcPort
		}
		return *peerPort
	}())

	httpAddr = net.JoinHostPort("localhost", func() string {
		if httpPort == nil {
			return defaultHttpPort
		}
		return *httpPort
	}())

	return grpcAddr, httpAddr
}

func main() {
	log := logger.NewLogger()
	grpcAddr, httpAddr := getAddress()

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.WithError(err).WithField("address", grpcAddr).Fatal("listen for grpc")
	}

	grpcServer := grpc.NewServer()
	defer grpcServer.GracefulStop()

	ctx := logger.SetContext(log)

	server, err := NewPeer(ctx, trackerAddr, grpcAddr)
	if err != nil {
		log.WithError(err).Fatal("cannot create peer")
	}

	api.RegisterPeerServer(grpcServer, server)

	mux := runtime.NewServeMux()
	err = api.RegisterPeerHandlerFromEndpoint(ctx, mux, grpcAddr, []grpc.DialOption{grpc.WithInsecure()})
	if err != nil {
		log.WithError(err).Fatal("cannot register")
	}

	srv := &http.Server{
		Addr:    httpAddr,
		Handler: mux,
	}

	group := errgroup.Group{}
	group.Go(func() error {
		log.WithField("grpc_address", grpcAddr).Info("start grpc server")
		return grpcServer.Serve(lis)
	})

	group.Go(func() error {
		log.WithField("http_address", httpAddr).Info("start http server")
		return srv.ListenAndServe()
	})

	err = group.Wait()
	if err != nil {
		log.WithError(err).Fatal("group wait")
	}
}
