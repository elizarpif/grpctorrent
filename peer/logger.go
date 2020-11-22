package main

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"
)

type logger struct {
}

func setContext() context.Context {
	return context.WithValue(context.Background(), logger{}, newLogger())
}

func newLogger() *logrus.Logger {
	log := logrus.StandardLogger()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	return log
}

func getLogger(ctx context.Context) *logrus.Logger {
	value := ctx.Value(logger{})
	log, ok := value.(*logrus.Logger)
	if !ok {
		log = newLogger()
	}

	return log
}
