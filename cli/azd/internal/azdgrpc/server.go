package azdgrpc

import (
	"fmt"
	"log"
	"net"

	azdext "github.com/azure/azure-dev/cli/azd/pkg/azdext/gen/grpc"
	"google.golang.org/grpc"
)

type ServerInfo struct {
	Address string
	Port    int
}

type Server struct {
	grpcServer         *grpc.Server
	greeterService     azdext.GreeterServer
	environmentService azdext.EnvironmentServiceServer
	promptService      azdext.PromptServiceServer
}

func NewServer(
	greeterService azdext.GreeterServer,
	environmentService azdext.EnvironmentServiceServer,
	promptService azdext.PromptServiceServer,
) *Server {
	return &Server{
		greeterService:     greeterService,
		environmentService: environmentService,
		promptService:      promptService,
		grpcServer:         grpc.NewServer(),
	}
}

func (s *Server) Start() (*ServerInfo, error) {
	// Use ":0" to let the system assign an available random port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}

	// Get the assigned random port
	randomPort := listener.Addr().(*net.TCPAddr).Port

	// Register the Greeter service with the gRPC server
	azdext.RegisterGreeterServer(s.grpcServer, s.greeterService)
	azdext.RegisterEnvironmentServiceServer(s.grpcServer, s.environmentService)
	azdext.RegisterPromptServiceServer(s.grpcServer, s.promptService)

	go func() {
		// Start the gRPC server
		if err := s.grpcServer.Serve(listener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	log.Printf("AZD Server listening on port %d", randomPort)

	return &ServerInfo{
		Address: fmt.Sprintf("localhost:%d", randomPort),
		Port:    randomPort,
	}, nil
}

func (s *Server) Stop() error {
	if s.grpcServer == nil {
		return fmt.Errorf("server is not running")
	}

	s.grpcServer.Stop()
	log.Println("AZD Server stopped")

	return nil
}
