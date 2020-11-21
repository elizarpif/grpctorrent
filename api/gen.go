//go:generate protoc -I =. --go_out=plugins=grpc:./ --grpc-gateway_out=logtostderr=true:./ torrent.proto
package api