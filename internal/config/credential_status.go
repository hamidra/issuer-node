package config

import (
	"fmt"
	"strings"
)

const (
	sparseMerkleTreeProof                 = "SparseMerkleTreeProof"
	iden3commRevocationStatusV1           = "Iden3commRevocationStatusV1.0"
	iden3ReverseSparseMerkleTreeProof     = "Iden3ReverseSparseMerkleTreeProof"
	iden3OnchainSparseMerkleTreeProof2023 = "Iden3OnchainSparseMerkleTreeProof2023"
	onChain                               = "OnChain"
	offChain                              = "OffChain"
	none                                  = "None"
)

// RHSMode is a mode of RHS
type RHSMode string

// CredentialStatus is the type of credential status
type CredentialStatus struct {
	DirectStatus         DirectStatus
	RHS                  RHS
	OnchainTreeStore     OnchainTreeStore `mapstructure:"OnchainTreeStore"`
	RHSMode              RHSMode          `tip:"Reverse hash service mode (OffChain, OnChain, Mixed, None)"`
	SingleIssuer         bool
	CredentialStatusType string `mapstructure:"CredentialStatusType" default:"Iden3commRevocationStatusV1"`
}

// DirectStatus is the type of direct status
type DirectStatus struct {
	URL string `mapstructure:"URL"`
}

// GetURL returns the URL of the di	rect status
func (r *DirectStatus) GetURL() string {
	return strings.TrimSuffix(r.URL, "/")
}

// GetAgentURL returns the URL of the agent endpoint
func (r *DirectStatus) GetAgentURL() string {
	return fmt.Sprintf("%s/v1/agent", strings.TrimSuffix(r.URL, "/"))
}

// RHS is the type of RHS
type RHS struct {
	URL string `mapstructure:"URL"`
}

// GetURL returns the URL of the RHS
func (r *RHS) GetURL() string {
	return strings.TrimSuffix(r.URL, "/")
}

// DIDResolver is the type of DID resolver
type DIDResolver struct {
	URL string `mapstructure:"URL"`
}

// GetURL returns the URL of the DID resolver
func (r *DIDResolver) GetURL() string {
	return strings.TrimSuffix(r.URL, "/")
}

// OnchainTreeStore is the type of onchain tree store
type OnchainTreeStore struct {
	SupportedTreeStoreContract string `mapstructure:"SupportedTreeStoreContract"`
	PublishingKeyPath          string `mapstructure:"PublishingKeyPath" default:"pbkey"`
	ChainID                    string `mapstructure:"ChainID"`
}
