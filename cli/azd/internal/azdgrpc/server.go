package azdgrpc

import (
	"fmt"
	"log"
	"net"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"google.golang.org/grpc"
)

type ServerInfo struct {
	Address string
	Port    int
}

type Server struct {
	grpcServer         *grpc.Server
	projectService     azdext.ProjectServiceServer
	environmentService azdext.EnvironmentServiceServer
	promptService      azdext.PromptServiceServer
	userConfigService  azdext.UserConfigServiceServer
	deploymentService  azdext.DeploymentServiceServer
}

func NewServer(
	projectService azdext.ProjectServiceServer,
	environmentService azdext.EnvironmentServiceServer,
	promptService azdext.PromptServiceServer,
	userConfigService azdext.UserConfigServiceServer,
	deploymentService azdext.DeploymentServiceServer,
) *Server {
	return &Server{
		projectService:     projectService,
		environmentService: environmentService,
		promptService:      promptService,
		userConfigService:  userConfigService,
		deploymentService:  deploymentService,
		grpcServer:         grpc.NewServer(),
	}
}

func (s *Server) Start() (*ServerInfo, error) {
	// Use ":0" to let the system assign an available random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}

	// Get the assigned random port
	randomPort := listener.Addr().(*net.TCPAddr).Port

	// Register the Greeter service with the gRPC server
	azdext.RegisterProjectServiceServer(s.grpcServer, s.projectService)
	azdext.RegisterEnvironmentServiceServer(s.grpcServer, s.environmentService)
	azdext.RegisterPromptServiceServer(s.grpcServer, s.promptService)
	azdext.RegisterUserConfigServiceServer(s.grpcServer, s.userConfigService)
	azdext.RegisterDeploymentServiceServer(s.grpcServer, s.deploymentService)

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
