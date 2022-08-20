// To regenerate the protocol files in this package:
//   - Download the latest protoc https://github.com/protocolbuffers/protobuf/releases
//   - Build with the usual `./configure && make` and ensure it's on your $PATH
//   - Delete all the .proto and .pb.go files, pull in fresh ones from Trezor
//   - Grab the latest Go plugin `go get -u google.golang.org/protobuf`
//   - Vendor in the latest Go plugin `govendor fetch github.com/golang/protobuf/...`
//go:generate protoc -I/usr/local/include:. --go_out=. messages.proto messages-common.proto messages-management.proto messages-entropy.proto

// Package trezor contains the wire protocol.
package trezor

import (
	"reflect"

	"github.com/golang/protobuf/proto"
)

// Type returns the protocol buffer type number of a specific message. If the
// message is nil, this method panics!
func Type(msg proto.Message) uint16 {
	return uint16(MessageType_value["MessageType_"+reflect.TypeOf(msg).Elem().Name()])
}

// Name returns the friendly message type name of a specific protocol buffer
// type number.
func Name(kind uint16) string {
	name := MessageType_name[int32(kind)]
	if len(name) < 12 {
		return name
	}
	return name[12:]
}
