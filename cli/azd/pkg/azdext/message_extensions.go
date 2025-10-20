package azdext

import "github.com/azure/azure-dev/cli/azd/pkg/grpc/streaming"

func (m *FrameworkServiceMessage) StreamingError() streaming.ErrorMessage {
	return m.GetError()
}

func (m *FrameworkServiceMessage) Progress() streaming.ProgressMessage {
	return m.GetProgressMessage()
}

// IsExpectedResponse checks if the given message is the expected response for this request
func (m *FrameworkServiceMessage) IsExpectedResponse(response streaming.StreamMessage) bool {
	resp, ok := response.(*FrameworkServiceMessage)
	if !ok {
		return false
	}

	// Check what type of request this message is, then verify the response matches
	switch m.MessageType.(type) {
	case *FrameworkServiceMessage_RestoreRequest:
		return resp.GetRestoreResponse() != nil
	case *FrameworkServiceMessage_BuildRequest:
		return resp.GetBuildResponse() != nil
	case *FrameworkServiceMessage_PackageRequest:
		return resp.GetPackageResponse() != nil
	case *FrameworkServiceMessage_RequirementsRequest:
		return resp.GetRequirementsResponse() != nil
	case *FrameworkServiceMessage_RequiredExternalToolsRequest:
		return resp.GetRequiredExternalToolsResponse() != nil
	case *FrameworkServiceMessage_InitializeRequest:
		return resp.GetInitializeResponse() != nil
	case *FrameworkServiceMessage_RegisterFrameworkServiceRequest:
		return resp.GetRegisterFrameworkServiceResponse() != nil
	default:
		// If this message is not a request, it cannot expect a response
		return false
	}
}

// ServiceTargetMessage extensions to implement StreamMessage interface

func (m *ServiceTargetMessage) StreamingError() streaming.ErrorMessage {
	return m.GetError()
}

func (m *ServiceTargetMessage) Progress() streaming.ProgressMessage {
	return m.GetProgressMessage()
}

// IsExpectedResponse checks if the given message is the expected response for this request
func (m *ServiceTargetMessage) IsExpectedResponse(response streaming.StreamMessage) bool {
	resp, ok := response.(*ServiceTargetMessage)
	if !ok {
		return false
	}

	// Check what type of request this message is, then verify the response matches
	switch m.MessageType.(type) {
	case *ServiceTargetMessage_RegisterServiceTargetRequest:
		return resp.GetRegisterServiceTargetResponse() != nil
	case *ServiceTargetMessage_InitializeRequest:
		return resp.GetInitializeResponse() != nil
	case *ServiceTargetMessage_GetTargetResourceRequest:
		return resp.GetGetTargetResourceResponse() != nil
	case *ServiceTargetMessage_DeployRequest:
		return resp.GetDeployResponse() != nil
	case *ServiceTargetMessage_PackageRequest:
		return resp.GetPackageResponse() != nil
	case *ServiceTargetMessage_PublishRequest:
		return resp.GetPublishResponse() != nil
	case *ServiceTargetMessage_EndpointsRequest:
		return resp.GetEndpointsResponse() != nil
	default:
		// If this message is not a request, it cannot expect a response
		return false
	}
}
