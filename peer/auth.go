package main

import "context"

type auth struct {
	data map[string]string
}

func newAuth(address string) *auth {
	a := make(map[string]string)
	a["address"] = address
	return &auth{data: a}
}

func (a *auth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return a.data, nil
}

func (a *auth) RequireTransportSecurity() bool {
	return false
}

