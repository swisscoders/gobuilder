package any

import (
    "fmt"
    "reflect"
    "github.com/golang/protobuf/proto"
    "../../../proto/google/protobuf"
)

const typeFormat = "type.googleapis.com/%s"

func PackFrom(p proto.Message) (*google_protobuf.Any, error) {
    data, err := proto.Marshal(p)
    if err != nil {
        return nil, err
    }

    return &google_protobuf.Any{
        TypeUrl: fmt.Sprintf(typeFormat, reflect.TypeOf(p).Name()),
        Value: data,
    }, nil
}

func UnpackTo(p *google_protobuf.Any, data proto.Message) error {
   return proto.Unmarshal(p.Value, data)   
}

func UnpackTextTo(p *google_protobuf.Any, data proto.Message) error {
   return proto.UnmarshalText(string(p.Value), data)   
}

func IsType(any *google_protobuf.Any, typ string) bool {
    return any.TypeUrl == fmt.Sprintf(typeFormat, typ)
}