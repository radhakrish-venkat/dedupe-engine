// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.6
// 	protoc        v5.29.3
// source: pkg/api/storage_service.proto

package api

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type StoreChunkRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Fingerprint   string                 `protobuf:"bytes,1,opt,name=fingerprint,proto3" json:"fingerprint,omitempty"` // Blake3 hash, used as object key
	ChunkData     []byte                 `protobuf:"bytes,2,opt,name=chunk_data,json=chunkData,proto3" json:"chunk_data,omitempty"`
	Size          int64                  `protobuf:"varint,3,opt,name=size,proto3" json:"size,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *StoreChunkRequest) Reset() {
	*x = StoreChunkRequest{}
	mi := &file_pkg_api_storage_service_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *StoreChunkRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StoreChunkRequest) ProtoMessage() {}

func (x *StoreChunkRequest) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_api_storage_service_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StoreChunkRequest.ProtoReflect.Descriptor instead.
func (*StoreChunkRequest) Descriptor() ([]byte, []int) {
	return file_pkg_api_storage_service_proto_rawDescGZIP(), []int{0}
}

func (x *StoreChunkRequest) GetFingerprint() string {
	if x != nil {
		return x.Fingerprint
	}
	return ""
}

func (x *StoreChunkRequest) GetChunkData() []byte {
	if x != nil {
		return x.ChunkData
	}
	return nil
}

func (x *StoreChunkRequest) GetSize() int64 {
	if x != nil {
		return x.Size
	}
	return 0
}

type StoreChunkResponse struct {
	state           protoimpl.MessageState `protogen:"open.v1"`
	StorageLocation string                 `protobuf:"bytes,1,opt,name=storage_location,json=storageLocation,proto3" json:"storage_location,omitempty"` // MinIO object key
	StorageNodeId   string                 `protobuf:"bytes,2,opt,name=storage_node_id,json=storageNodeId,proto3" json:"storage_node_id,omitempty"`
	Success         bool                   `protobuf:"varint,3,opt,name=success,proto3" json:"success,omitempty"`
	ErrorMessage    string                 `protobuf:"bytes,4,opt,name=error_message,json=errorMessage,proto3" json:"error_message,omitempty"`
	unknownFields   protoimpl.UnknownFields
	sizeCache       protoimpl.SizeCache
}

func (x *StoreChunkResponse) Reset() {
	*x = StoreChunkResponse{}
	mi := &file_pkg_api_storage_service_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *StoreChunkResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StoreChunkResponse) ProtoMessage() {}

func (x *StoreChunkResponse) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_api_storage_service_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StoreChunkResponse.ProtoReflect.Descriptor instead.
func (*StoreChunkResponse) Descriptor() ([]byte, []int) {
	return file_pkg_api_storage_service_proto_rawDescGZIP(), []int{1}
}

func (x *StoreChunkResponse) GetStorageLocation() string {
	if x != nil {
		return x.StorageLocation
	}
	return ""
}

func (x *StoreChunkResponse) GetStorageNodeId() string {
	if x != nil {
		return x.StorageNodeId
	}
	return ""
}

func (x *StoreChunkResponse) GetSuccess() bool {
	if x != nil {
		return x.Success
	}
	return false
}

func (x *StoreChunkResponse) GetErrorMessage() string {
	if x != nil {
		return x.ErrorMessage
	}
	return ""
}

type GetChunkRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Fingerprint   string                 `protobuf:"bytes,1,opt,name=fingerprint,proto3" json:"fingerprint,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetChunkRequest) Reset() {
	*x = GetChunkRequest{}
	mi := &file_pkg_api_storage_service_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetChunkRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetChunkRequest) ProtoMessage() {}

func (x *GetChunkRequest) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_api_storage_service_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetChunkRequest.ProtoReflect.Descriptor instead.
func (*GetChunkRequest) Descriptor() ([]byte, []int) {
	return file_pkg_api_storage_service_proto_rawDescGZIP(), []int{2}
}

func (x *GetChunkRequest) GetFingerprint() string {
	if x != nil {
		return x.Fingerprint
	}
	return ""
}

type GetChunkResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	ChunkData     []byte                 `protobuf:"bytes,1,opt,name=chunk_data,json=chunkData,proto3" json:"chunk_data,omitempty"`
	Size          int64                  `protobuf:"varint,2,opt,name=size,proto3" json:"size,omitempty"`
	Found         bool                   `protobuf:"varint,3,opt,name=found,proto3" json:"found,omitempty"`
	ErrorMessage  string                 `protobuf:"bytes,4,opt,name=error_message,json=errorMessage,proto3" json:"error_message,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetChunkResponse) Reset() {
	*x = GetChunkResponse{}
	mi := &file_pkg_api_storage_service_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetChunkResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetChunkResponse) ProtoMessage() {}

func (x *GetChunkResponse) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_api_storage_service_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetChunkResponse.ProtoReflect.Descriptor instead.
func (*GetChunkResponse) Descriptor() ([]byte, []int) {
	return file_pkg_api_storage_service_proto_rawDescGZIP(), []int{3}
}

func (x *GetChunkResponse) GetChunkData() []byte {
	if x != nil {
		return x.ChunkData
	}
	return nil
}

func (x *GetChunkResponse) GetSize() int64 {
	if x != nil {
		return x.Size
	}
	return 0
}

func (x *GetChunkResponse) GetFound() bool {
	if x != nil {
		return x.Found
	}
	return false
}

func (x *GetChunkResponse) GetErrorMessage() string {
	if x != nil {
		return x.ErrorMessage
	}
	return ""
}

var File_pkg_api_storage_service_proto protoreflect.FileDescriptor

const file_pkg_api_storage_service_proto_rawDesc = "" +
	"\n" +
	"\x1dpkg/api/storage_service.proto\x12\x0fstorage_service\"h\n" +
	"\x11StoreChunkRequest\x12 \n" +
	"\vfingerprint\x18\x01 \x01(\tR\vfingerprint\x12\x1d\n" +
	"\n" +
	"chunk_data\x18\x02 \x01(\fR\tchunkData\x12\x12\n" +
	"\x04size\x18\x03 \x01(\x03R\x04size\"\xa6\x01\n" +
	"\x12StoreChunkResponse\x12)\n" +
	"\x10storage_location\x18\x01 \x01(\tR\x0fstorageLocation\x12&\n" +
	"\x0fstorage_node_id\x18\x02 \x01(\tR\rstorageNodeId\x12\x18\n" +
	"\asuccess\x18\x03 \x01(\bR\asuccess\x12#\n" +
	"\rerror_message\x18\x04 \x01(\tR\ferrorMessage\"3\n" +
	"\x0fGetChunkRequest\x12 \n" +
	"\vfingerprint\x18\x01 \x01(\tR\vfingerprint\"\x80\x01\n" +
	"\x10GetChunkResponse\x12\x1d\n" +
	"\n" +
	"chunk_data\x18\x01 \x01(\fR\tchunkData\x12\x12\n" +
	"\x04size\x18\x02 \x01(\x03R\x04size\x12\x14\n" +
	"\x05found\x18\x03 \x01(\bR\x05found\x12#\n" +
	"\rerror_message\x18\x04 \x01(\tR\ferrorMessage2\xb8\x01\n" +
	"\x0eStorageService\x12U\n" +
	"\n" +
	"StoreChunk\x12\".storage_service.StoreChunkRequest\x1a#.storage_service.StoreChunkResponse\x12O\n" +
	"\bGetChunk\x12 .storage_service.GetChunkRequest\x1a!.storage_service.GetChunkResponseB7Z5github.com/radhakrishnan.venkat/dedupe-engine/pkg/apib\x06proto3"

var (
	file_pkg_api_storage_service_proto_rawDescOnce sync.Once
	file_pkg_api_storage_service_proto_rawDescData []byte
)

func file_pkg_api_storage_service_proto_rawDescGZIP() []byte {
	file_pkg_api_storage_service_proto_rawDescOnce.Do(func() {
		file_pkg_api_storage_service_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_pkg_api_storage_service_proto_rawDesc), len(file_pkg_api_storage_service_proto_rawDesc)))
	})
	return file_pkg_api_storage_service_proto_rawDescData
}

var file_pkg_api_storage_service_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_pkg_api_storage_service_proto_goTypes = []any{
	(*StoreChunkRequest)(nil),  // 0: storage_service.StoreChunkRequest
	(*StoreChunkResponse)(nil), // 1: storage_service.StoreChunkResponse
	(*GetChunkRequest)(nil),    // 2: storage_service.GetChunkRequest
	(*GetChunkResponse)(nil),   // 3: storage_service.GetChunkResponse
}
var file_pkg_api_storage_service_proto_depIdxs = []int32{
	0, // 0: storage_service.StorageService.StoreChunk:input_type -> storage_service.StoreChunkRequest
	2, // 1: storage_service.StorageService.GetChunk:input_type -> storage_service.GetChunkRequest
	1, // 2: storage_service.StorageService.StoreChunk:output_type -> storage_service.StoreChunkResponse
	3, // 3: storage_service.StorageService.GetChunk:output_type -> storage_service.GetChunkResponse
	2, // [2:4] is the sub-list for method output_type
	0, // [0:2] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_pkg_api_storage_service_proto_init() }
func file_pkg_api_storage_service_proto_init() {
	if File_pkg_api_storage_service_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_pkg_api_storage_service_proto_rawDesc), len(file_pkg_api_storage_service_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_pkg_api_storage_service_proto_goTypes,
		DependencyIndexes: file_pkg_api_storage_service_proto_depIdxs,
		MessageInfos:      file_pkg_api_storage_service_proto_msgTypes,
	}.Build()
	File_pkg_api_storage_service_proto = out.File
	file_pkg_api_storage_service_proto_goTypes = nil
	file_pkg_api_storage_service_proto_depIdxs = nil
}
