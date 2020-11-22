//go:generate protoc -I =. --go_out=plugins=grpc:./ --swagger_out=logtostderr=true:./ --grpc-gateway_out=logtostderr=true:./ torrent.proto
package api