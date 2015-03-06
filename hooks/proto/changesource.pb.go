// Code generated by protoc-gen-go.
// source: changesource.proto
// DO NOT EDIT!

/*
Package proto is a generated protocol buffer package.

It is generated from these files:
	changesource.proto

It has these top-level messages:
	ChangeRequest
	ChangeResponse
*/
package proto

import proto1 "github.com/golang/protobuf/proto"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto1.Marshal

type ChangeRequest struct {
	Commithash string   `protobuf:"bytes,1,opt,name=commithash" json:"commithash,omitempty"`
	Branch     string   `protobuf:"bytes,2,opt,name=branch" json:"branch,omitempty"`
	Dir        string   `protobuf:"bytes,3,opt,name=dir" json:"dir,omitempty"`
	Files      []string `protobuf:"bytes,4,rep,name=files" json:"files,omitempty"`
}

func (m *ChangeRequest) Reset()         { *m = ChangeRequest{} }
func (m *ChangeRequest) String() string { return proto1.CompactTextString(m) }
func (*ChangeRequest) ProtoMessage()    {}

type ChangeResponse struct {
}

func (m *ChangeResponse) Reset()         { *m = ChangeResponse{} }
func (m *ChangeResponse) String() string { return proto1.CompactTextString(m) }
func (*ChangeResponse) ProtoMessage()    {}

func init() {
}

// Client API for ChangeSource service

type ChangeSourceClient interface {
	Notify(ctx context.Context, in *ChangeRequest, opts ...grpc.CallOption) (*ChangeResponse, error)
}

type changeSourceClient struct {
	cc *grpc.ClientConn
}

func NewChangeSourceClient(cc *grpc.ClientConn) ChangeSourceClient {
	return &changeSourceClient{cc}
}

func (c *changeSourceClient) Notify(ctx context.Context, in *ChangeRequest, opts ...grpc.CallOption) (*ChangeResponse, error) {
	out := new(ChangeResponse)
	err := grpc.Invoke(ctx, "/proto.ChangeSource/Notify", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for ChangeSource service

type ChangeSourceServer interface {
	Notify(context.Context, *ChangeRequest) (*ChangeResponse, error)
}

func RegisterChangeSourceServer(s *grpc.Server, srv ChangeSourceServer) {
	s.RegisterService(&_ChangeSource_serviceDesc, srv)
}

func _ChangeSource_Notify_Handler(srv interface{}, ctx context.Context, buf []byte) (proto1.Message, error) {
	in := new(ChangeRequest)
	if err := proto1.Unmarshal(buf, in); err != nil {
		return nil, err
	}
	out, err := srv.(ChangeSourceServer).Notify(ctx, in)
	if err != nil {
		return nil, err
	}
	return out, nil
}

var _ChangeSource_serviceDesc = grpc.ServiceDesc{
	ServiceName: "proto.ChangeSource",
	HandlerType: (*ChangeSourceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Notify",
			Handler:    _ChangeSource_Notify_Handler,
		},
	},
	Streams: []grpc.StreamDesc{},
}
