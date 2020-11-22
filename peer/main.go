package main

import (
	"context"
	"net"
	"os"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	"github.com/elizarpif/grpctorrent/api"
	"github.com/sirupsen/logrus"
)

func setContext() context.Context {
	return context.WithValue(context.Background(), "logger", newLogger())
}

func newLogger() *logrus.Logger {
	log := logrus.StandardLogger()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	return log
}

func getLogger(ctx context.Context) *logrus.Logger {
	value := ctx.Value("logger")
	log, ok := value.(*logrus.Logger)
	if !ok {
		log = newLogger()
	}

	return log
}

const (
	grpcAddress = "localhost:9001"
	trackerAddr = "localhost:9000"
)

func main() {
	logger := newLogger()

	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		logger.WithError(err).WithField("address", grpcAddress).Fatal("listen for grpc")
	}

	grpcServer := grpc.NewServer()
	defer grpcServer.GracefulStop()

	ctx := setContext()
	server, err := NewPeer(ctx, trackerAddr)
	if err != nil{
		logger.WithError(err).Fatal("cannot create peer")
	}

	api.RegisterPeerServer(grpcServer, server)

	group := errgroup.Group{}
	group.Go(func() error {
		logger.WithField("address", grpcAddress).Info("start grpc server")
		return grpcServer.Serve(lis)
	})

	group.Go(func() error {
		err = server.UploadFileToTracker(ctx, "some.txt")
		if err != nil{
			logger.WithError(err).Fatal()
		}
		return err
	})

	err = group.Wait()
	if err != nil {
		logger.WithError(err).Fatal("group wait")
	}
}
