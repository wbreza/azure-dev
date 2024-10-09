// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package azure

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

type DeploymentScope string

const DeploymentScopeSubscription DeploymentScope = "subscription"
const DeploymentScopeResourceGroup DeploymentScope = "resourceGroup"

// RawArmTemplate is a JSON encoded ARM template.
type RawArmTemplate = json.RawMessage

// ArmTemplate represents an Azure Resource Manager deployment template. It follows the structure outlined
// at https://learn.microsoft.com/azure/azure-resource-manager/templates/syntax, but only exposes portions of the
// object that azd cares about.
type ArmTemplate struct {
	Schema         string                          `json:"$schema"`
	ContentVersion string                          `json:"contentVersion"`
	Parameters     ArmTemplateParameterDefinitions `json:"parameters"`
	Outputs        ArmTemplateOutputs              `json:"outputs"`
	Definitions    ArmTemplateParameterDefinitions `json:"definitions"`
}

// TargetScope uses the $schema property of the template to determine what scope this template should be deployed
// at or an error if the scope could not be determined.
func (t ArmTemplate) TargetScope() (DeploymentScope, error) {
	if t.Schema == "" {
		return DeploymentScope(""), errors.New("no schema in template")
	}

	u, err := url.Parse(t.Schema)
	if err != nil {
		return DeploymentScope(""), fmt.Errorf("error parsing schema: %w", err)
	}

	switch {
	case strings.EqualFold(path.Base(u.Path), "subscriptionDeploymentTemplate.json"):
		return DeploymentScopeSubscription, nil
	case strings.EqualFold(path.Base(u.Path), "deploymentTemplate.json"):
		return DeploymentScopeResourceGroup, nil
	default:
		return DeploymentScope(""), fmt.Errorf("unknown schema: %s", t.Schema)
	}
}

type ArmTemplateParameterDefinitions map[string]ArmTemplateParameterDefinition

type ArmTemplateOutputs map[string]ArmTemplateOutput

type ArmTemplateParameterAdditionalProperties struct {
	Type      string                     `json:"type"`
	MinValue  *int                       `json:"minValue,omitempty"`
	MaxValue  *int                       `json:"maxValue,omitempty"`
	MinLength *int                       `json:"minLength,omitempty"`
	MaxLength *int                       `json:"maxLength,omitempty"`
	Metadata  map[string]json.RawMessage `json:"metadata"`
}

type ArmTemplateParameterDefinition struct {
	Type                 string                                   `json:"type"`
	DefaultValue         any                                      `json:"defaultValue"`
	AllowedValues        *[]any                                   `json:"allowedValues,omitempty"`
	MinValue             *int                                     `json:"minValue,omitempty"`
	MaxValue             *int                                     `json:"maxValue,omitempty"`
	MinLength            *int                                     `json:"minLength,omitempty"`
	MaxLength            *int                                     `json:"maxLength,omitempty"`
	Metadata             map[string]json.RawMessage               `json:"metadata"`
	Ref                  string                                   `json:"$ref"`
	Properties           ArmTemplateParameterDefinitions          `json:"properties,omitempty"`
	AdditionalProperties ArmTemplateParameterAdditionalProperties `json:"additionalProperties,omitempty"`
}

func (d *ArmTemplateParameterDefinition) Secure() bool {
	lowerCase := strings.ToLower(d.Type)
	return lowerCase == "secureobject" || lowerCase == "securestring"
}

type AutoGenInput struct {
	Length     uint  `json:"length,omitempty"`
	NoLower    *bool `json:"noLower,omitempty"`
	NoUpper    *bool `json:"noUpper,omitempty"`
	NoNumeric  *bool `json:"noNumeric,omitempty"`
	NoSpecial  *bool `json:"noSpecial,omitempty"`
	MinLower   *uint `json:"minLower,omitempty"`
	MinUpper   *uint `json:"minUpper,omitempty"`
	MinNumeric *uint `json:"minNumeric,omitempty"`
	MinSpecial *uint `json:"minSpecial,omitempty"`
}

// ResourceInputMetadata is set on ARM/Bicep parameter properties
// This metadata is used to generate a resource picker in the CLI
type ResourceInputMetadata struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

// OptionalResource is used to represent a resource that may or may not exist in the Azure subscription.
// This value is used as an input value to the ARM/Bicep parameter for parameters with azd type resource metadata.
type OptionalResource struct {
	Name           string `json:"name"`
	SubscriptionId string `json:"subscriptionId"`
	ResourceGroup  string `json:"resourceGroup"`
	Exists         bool   `json:"exists"`
}

type AzdMetadataType string

const AzdMetadataTypeLocation AzdMetadataType = "location"
const AzdMetadataTypeGenerate AzdMetadataType = "generate"
const AzdMetadataTypeGenerateOrManual AzdMetadataType = "generateOrManual"
const AzdMetadataTypeResource AzdMetadataType = "resource"

type AzdMetadata struct {
	Type               *AzdMetadataType       `json:"type,omitempty"`
	AutoGenerateConfig *AutoGenInput          `json:"config,omitempty"`
	DefaultValueExpr   *string                `json:"defaultValueExpr,omitempty"`
	Resource           *ResourceInputMetadata `json:"resource,omitempty"`
}

// Description returns the value of the "Description" string metadata for this parameter or empty if it can not be found.
func (p ArmTemplateParameterDefinition) Description() (string, bool) {
	if v, has := p.Metadata["description"]; has {
		var description string
		if err := json.Unmarshal(v, &description); err == nil {
			return description, true
		}
	}

	return "", false
}

// AzdMetadata returns the value of the "azd" object metadata for this parameter or the zero value if it can not be found.
func (p ArmTemplateParameterDefinition) AzdMetadata() (AzdMetadata, bool) {
	if v, has := p.Metadata["azd"]; has {
		var metadata AzdMetadata
		if err := json.Unmarshal(v, &metadata); err == nil {
			return metadata, true
		}
	}

	return AzdMetadata{}, false
}

type ArmTemplateOutput struct {
	Type     string         `json:"type"`
	Value    any            `json:"value"`
	Metadata map[string]any `json:"metadata"`
	Ref      string         `json:"$ref"`
}
