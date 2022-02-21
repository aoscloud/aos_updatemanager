// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package iamanager

import (
	context "context"
	empty "github.com/golang/protobuf/ptypes/empty"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// IAMPublicServiceClient is the client API for IAMPublicService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type IAMPublicServiceClient interface {
	GetSystemInfo(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*SystemInfo, error)
	GetCertTypes(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*CertTypes, error)
	GetCert(ctx context.Context, in *GetCertRequest, opts ...grpc.CallOption) (*GetCertResponse, error)
	GetPermissions(ctx context.Context, in *PermissionsRequest, opts ...grpc.CallOption) (*PermissionsResponse, error)
	GetUsers(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*Users, error)
	SubscribeUsersChanged(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (IAMPublicService_SubscribeUsersChangedClient, error)
}

type iAMPublicServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewIAMPublicServiceClient(cc grpc.ClientConnInterface) IAMPublicServiceClient {
	return &iAMPublicServiceClient{cc}
}

func (c *iAMPublicServiceClient) GetSystemInfo(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*SystemInfo, error) {
	out := new(SystemInfo)
	err := c.cc.Invoke(ctx, "/iamanager.v2.IAMPublicService/GetSystemInfo", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *iAMPublicServiceClient) GetCertTypes(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*CertTypes, error) {
	out := new(CertTypes)
	err := c.cc.Invoke(ctx, "/iamanager.v2.IAMPublicService/GetCertTypes", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *iAMPublicServiceClient) GetCert(ctx context.Context, in *GetCertRequest, opts ...grpc.CallOption) (*GetCertResponse, error) {
	out := new(GetCertResponse)
	err := c.cc.Invoke(ctx, "/iamanager.v2.IAMPublicService/GetCert", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *iAMPublicServiceClient) GetPermissions(ctx context.Context, in *PermissionsRequest, opts ...grpc.CallOption) (*PermissionsResponse, error) {
	out := new(PermissionsResponse)
	err := c.cc.Invoke(ctx, "/iamanager.v2.IAMPublicService/GetPermissions", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *iAMPublicServiceClient) GetUsers(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*Users, error) {
	out := new(Users)
	err := c.cc.Invoke(ctx, "/iamanager.v2.IAMPublicService/GetUsers", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *iAMPublicServiceClient) SubscribeUsersChanged(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (IAMPublicService_SubscribeUsersChangedClient, error) {
	stream, err := c.cc.NewStream(ctx, &IAMPublicService_ServiceDesc.Streams[0], "/iamanager.v2.IAMPublicService/SubscribeUsersChanged", opts...)
	if err != nil {
		return nil, err
	}
	x := &iAMPublicServiceSubscribeUsersChangedClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type IAMPublicService_SubscribeUsersChangedClient interface {
	Recv() (*Users, error)
	grpc.ClientStream
}

type iAMPublicServiceSubscribeUsersChangedClient struct {
	grpc.ClientStream
}

func (x *iAMPublicServiceSubscribeUsersChangedClient) Recv() (*Users, error) {
	m := new(Users)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// IAMPublicServiceServer is the server API for IAMPublicService service.
// All implementations must embed UnimplementedIAMPublicServiceServer
// for forward compatibility
type IAMPublicServiceServer interface {
	GetSystemInfo(context.Context, *empty.Empty) (*SystemInfo, error)
	GetCertTypes(context.Context, *empty.Empty) (*CertTypes, error)
	GetCert(context.Context, *GetCertRequest) (*GetCertResponse, error)
	GetPermissions(context.Context, *PermissionsRequest) (*PermissionsResponse, error)
	GetUsers(context.Context, *empty.Empty) (*Users, error)
	SubscribeUsersChanged(*empty.Empty, IAMPublicService_SubscribeUsersChangedServer) error
	mustEmbedUnimplementedIAMPublicServiceServer()
}

// UnimplementedIAMPublicServiceServer must be embedded to have forward compatible implementations.
type UnimplementedIAMPublicServiceServer struct {
}

func (UnimplementedIAMPublicServiceServer) GetSystemInfo(context.Context, *empty.Empty) (*SystemInfo, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetSystemInfo not implemented")
}
func (UnimplementedIAMPublicServiceServer) GetCertTypes(context.Context, *empty.Empty) (*CertTypes, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetCertTypes not implemented")
}
func (UnimplementedIAMPublicServiceServer) GetCert(context.Context, *GetCertRequest) (*GetCertResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetCert not implemented")
}
func (UnimplementedIAMPublicServiceServer) GetPermissions(context.Context, *PermissionsRequest) (*PermissionsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetPermissions not implemented")
}
func (UnimplementedIAMPublicServiceServer) GetUsers(context.Context, *empty.Empty) (*Users, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetUsers not implemented")
}
func (UnimplementedIAMPublicServiceServer) SubscribeUsersChanged(*empty.Empty, IAMPublicService_SubscribeUsersChangedServer) error {
	return status.Errorf(codes.Unimplemented, "method SubscribeUsersChanged not implemented")
}
func (UnimplementedIAMPublicServiceServer) mustEmbedUnimplementedIAMPublicServiceServer() {}

// UnsafeIAMPublicServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to IAMPublicServiceServer will
// result in compilation errors.
type UnsafeIAMPublicServiceServer interface {
	mustEmbedUnimplementedIAMPublicServiceServer()
}

func RegisterIAMPublicServiceServer(s grpc.ServiceRegistrar, srv IAMPublicServiceServer) {
	s.RegisterService(&IAMPublicService_ServiceDesc, srv)
}

func _IAMPublicService_GetSystemInfo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(empty.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IAMPublicServiceServer).GetSystemInfo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/iamanager.v2.IAMPublicService/GetSystemInfo",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IAMPublicServiceServer).GetSystemInfo(ctx, req.(*empty.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _IAMPublicService_GetCertTypes_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(empty.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IAMPublicServiceServer).GetCertTypes(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/iamanager.v2.IAMPublicService/GetCertTypes",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IAMPublicServiceServer).GetCertTypes(ctx, req.(*empty.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _IAMPublicService_GetCert_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetCertRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IAMPublicServiceServer).GetCert(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/iamanager.v2.IAMPublicService/GetCert",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IAMPublicServiceServer).GetCert(ctx, req.(*GetCertRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _IAMPublicService_GetPermissions_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PermissionsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IAMPublicServiceServer).GetPermissions(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/iamanager.v2.IAMPublicService/GetPermissions",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IAMPublicServiceServer).GetPermissions(ctx, req.(*PermissionsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _IAMPublicService_GetUsers_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(empty.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IAMPublicServiceServer).GetUsers(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/iamanager.v2.IAMPublicService/GetUsers",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IAMPublicServiceServer).GetUsers(ctx, req.(*empty.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _IAMPublicService_SubscribeUsersChanged_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(empty.Empty)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(IAMPublicServiceServer).SubscribeUsersChanged(m, &iAMPublicServiceSubscribeUsersChangedServer{stream})
}

type IAMPublicService_SubscribeUsersChangedServer interface {
	Send(*Users) error
	grpc.ServerStream
}

type iAMPublicServiceSubscribeUsersChangedServer struct {
	grpc.ServerStream
}

func (x *iAMPublicServiceSubscribeUsersChangedServer) Send(m *Users) error {
	return x.ServerStream.SendMsg(m)
}

// IAMPublicService_ServiceDesc is the grpc.ServiceDesc for IAMPublicService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var IAMPublicService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "iamanager.v2.IAMPublicService",
	HandlerType: (*IAMPublicServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetSystemInfo",
			Handler:    _IAMPublicService_GetSystemInfo_Handler,
		},
		{
			MethodName: "GetCertTypes",
			Handler:    _IAMPublicService_GetCertTypes_Handler,
		},
		{
			MethodName: "GetCert",
			Handler:    _IAMPublicService_GetCert_Handler,
		},
		{
			MethodName: "GetPermissions",
			Handler:    _IAMPublicService_GetPermissions_Handler,
		},
		{
			MethodName: "GetUsers",
			Handler:    _IAMPublicService_GetUsers_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "SubscribeUsersChanged",
			Handler:       _IAMPublicService_SubscribeUsersChanged_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "iamanager/v2/iamanagerpublic.proto",
}
