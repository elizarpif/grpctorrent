package main

import (
	"context"
	"net"
	"os"

	"github.com/elizarpif/grpctorrent/api"
	"github.com/sirupsen/logrus"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const grpcAddress = "localhost:9000"

func setContext() context.Context {
	return context.WithValue(context.Background(), "logger", newLogger())
}

func newLogger() *logrus.Logger {
	log := logrus.StandardLogger()
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(logrus.DebugLevel)

	return log
}

//noinspection GoUnresolvedReference
func getLogger(ctx context.Context) *logrus.Logger {
	value := ctx.Value("logger")
	log, ok := value.(*logrus.Logger)
	if !ok {
		log = newLogger()
	}

	return log
}

func main() {
	logger := newLogger()

	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		logger.WithError(err).WithField("address", grpcAddress).Fatal("listen for grpc")
	}

	grpcServer := grpc.NewServer()
	defer grpcServer.GracefulStop()

	server := NewServer()
	api.RegisterTrackerServer(grpcServer, server)

	group := errgroup.Group{}
	group.Go(func() error {
		logger.WithField("address", grpcAddress).Info("start grpc server")
		return grpcServer.Serve(lis)
	})

	err = group.Wait()
	if err != nil {
		logger.WithError(err).Fatal("group wait")
	}
}
