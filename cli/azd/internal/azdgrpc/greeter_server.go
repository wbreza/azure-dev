package azdgrpc

import (
	"context"
	"fmt"

	azdext "github.com/azure/azure-dev/cli/azd/pkg/azdext/gen/grpc"
)

type greeterServer struct {
	azdext.UnimplementedGreeterServer
}

func NewGreeterServer() azdext.GreeterServer {
	return &greeterServer{}
}

func (s *greeterServer) SayHello(ctx context.Context, in *azdext.HelloRequest) (*azdext.HelloReply, error) {
	fmt.Println("Received: ", in.Name)

	return &azdext.HelloReply{
		Message: "Hello " + in.Name,
	}, nil
}
