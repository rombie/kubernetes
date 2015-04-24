/*
Copyright 2014 Google Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clientcmd

import (
	"strconv"

	"github.com/spf13/pflag"

	clientcmdapi "github.com/GoogleCloudPlatform/kubernetes/pkg/client/clientcmd/api"
)

// ConfigOverrides holds values that should override whatever information is pulled from the actual Config object.  You can't
// simply use an actual Config object, because Configs hold maps, but overrides are restricted to "at most one"
type ConfigOverrides struct {
	AuthInfo       clientcmdapi.AuthInfo
	ClusterInfo    clientcmdapi.Cluster
	Context        clientcmdapi.Context
	CurrentContext string
}

// ConfigOverrideFlags holds the flag names to be used for binding command line flags.  Notice that this structure tightly
// corresponds to ConfigOverrides
type ConfigOverrideFlags struct {
	AuthOverrideFlags    AuthOverrideFlags
	ClusterOverrideFlags ClusterOverrideFlags
	ContextOverrideFlags ContextOverrideFlags
	CurrentContext       FlagInfo
}

// AuthOverrideFlags holds the flag names to be used for binding command line flags for AuthInfo objects
type AuthOverrideFlags struct {
	AuthPath          FlagInfo
	ClientCertificate FlagInfo
	ClientKey         FlagInfo
	Token             FlagInfo
	Username          FlagInfo
	Password          FlagInfo
}

// ContextOverrideFlags holds the flag names to be used for binding command line flags for Cluster objects
type ContextOverrideFlags struct {
	ClusterName  FlagInfo
	AuthInfoName FlagInfo
	Namespace    FlagInfo
}

// ClusterOverride holds the flag names to be used for binding command line flags for Cluster objects
type ClusterOverrideFlags struct {
	APIServer             FlagInfo
	APIVersion            FlagInfo
	CertificateAuthority  FlagInfo
	InsecureSkipTLSVerify FlagInfo
}

type FlagInfo struct {
	LongName    string
	ShortName   string
	Default     string
	Description string
}

const (
	FlagClusterName  = "cluster"
	FlagAuthInfoName = "user"
	FlagContext      = "context"
	FlagNamespace    = "namespace"
	FlagAPIServer    = "server"
	FlagAPIVersion   = "api-version"
	FlagAuthPath     = "auth-path"
	FlagInsecure     = "insecure-skip-tls-verify"
	FlagCertFile     = "client-certificate"
	FlagKeyFile      = "client-key"
	FlagCAFile       = "certificate-authority"
	FlagEmbedCerts   = "embed-certs"
	FlagBearerToken  = "token"
	FlagUsername     = "username"
	FlagPassword     = "password"
)

// RecommendedAuthOverrideFlags is a convenience method to return recommended flag names prefixed with a string of your choosing
func RecommendedAuthOverrideFlags(prefix string) AuthOverrideFlags {
	return AuthOverrideFlags{
		AuthPath:          FlagInfo{prefix + FlagAuthPath, "", "", "Path to the auth info file. If missing, prompt the user. Only used if using https."},
		ClientCertificate: FlagInfo{prefix + FlagCertFile, "", "", "Path to a client key file for TLS."},
		ClientKey:         FlagInfo{prefix + FlagKeyFile, "", "", "Path to a client key file for TLS."},
		Token:             FlagInfo{prefix + FlagBearerToken, "", "", "Bearer token for authentication to the API server."},
		Username:          FlagInfo{prefix + FlagUsername, "", "", "Username for basic authentication to the API server."},
		Password:          FlagInfo{prefix + FlagPassword, "", "", "Password for basic authentication to the API server."},
	}
}

// RecommendedClusterOverrideFlags is a convenience method to return recommended flag names prefixed with a string of your choosing
func RecommendedClusterOverrideFlags(prefix string) ClusterOverrideFlags {
	return ClusterOverrideFlags{
		APIServer:             FlagInfo{prefix + FlagAPIServer, "", "", "The address and port of the Kubernetes API server"},
		APIVersion:            FlagInfo{prefix + FlagAPIVersion, "", "", "The API version to use when talking to the server"},
		CertificateAuthority:  FlagInfo{prefix + FlagCAFile, "", "", "Path to a cert. file for the certificate authority."},
		InsecureSkipTLSVerify: FlagInfo{prefix + FlagInsecure, "", "false", "If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure."},
	}
}

// RecommendedConfigOverrideFlags is a convenience method to return recommended flag names prefixed with a string of your choosing
func RecommendedConfigOverrideFlags(prefix string) ConfigOverrideFlags {
	return ConfigOverrideFlags{
		AuthOverrideFlags:    RecommendedAuthOverrideFlags(prefix),
		ClusterOverrideFlags: RecommendedClusterOverrideFlags(prefix),
		ContextOverrideFlags: RecommendedContextOverrideFlags(prefix),
		CurrentContext:       FlagInfo{prefix + FlagContext, "", "", "The name of the kubeconfig context to use"},
	}
}

// RecommendedContextOverrideFlags is a convenience method to return recommended flag names prefixed with a string of your choosing
func RecommendedContextOverrideFlags(prefix string) ContextOverrideFlags {
	return ContextOverrideFlags{
		ClusterName:  FlagInfo{prefix + FlagClusterName, "", "", "The name of the kubeconfig cluster to use"},
		AuthInfoName: FlagInfo{prefix + FlagAuthInfoName, "", "", "The name of the kubeconfig user to use"},
		Namespace:    FlagInfo{prefix + FlagNamespace, "", "", "If present, the namespace scope for this CLI request."},
	}
}

// BindAuthInfoFlags is a convenience method to bind the specified flags to their associated variables
func BindAuthInfoFlags(authInfo *clientcmdapi.AuthInfo, flags *pflag.FlagSet, flagNames AuthOverrideFlags) {
	bindStringFlag(flags, &authInfo.AuthPath, flagNames.AuthPath)
	bindStringFlag(flags, &authInfo.ClientCertificate, flagNames.ClientCertificate)
	bindStringFlag(flags, &authInfo.ClientKey, flagNames.ClientKey)
	bindStringFlag(flags, &authInfo.Token, flagNames.Token)
	bindStringFlag(flags, &authInfo.Username, flagNames.Username)
	bindStringFlag(flags, &authInfo.Password, flagNames.Password)
}

// BindClusterFlags is a convenience method to bind the specified flags to their associated variables
func BindClusterFlags(clusterInfo *clientcmdapi.Cluster, flags *pflag.FlagSet, flagNames ClusterOverrideFlags) {
	bindStringFlag(flags, &clusterInfo.Server, flagNames.APIServer)
	bindStringFlag(flags, &clusterInfo.APIVersion, flagNames.APIVersion)
	bindStringFlag(flags, &clusterInfo.CertificateAuthority, flagNames.CertificateAuthority)
	bindBoolFlag(flags, &clusterInfo.InsecureSkipTLSVerify, flagNames.InsecureSkipTLSVerify)
}

// BindOverrideFlags is a convenience method to bind the specified flags to their associated variables
func BindOverrideFlags(overrides *ConfigOverrides, flags *pflag.FlagSet, flagNames ConfigOverrideFlags) {
	BindAuthInfoFlags(&overrides.AuthInfo, flags, flagNames.AuthOverrideFlags)
	BindClusterFlags(&overrides.ClusterInfo, flags, flagNames.ClusterOverrideFlags)
	BindContextFlags(&overrides.Context, flags, flagNames.ContextOverrideFlags)
	bindStringFlag(flags, &overrides.CurrentContext, flagNames.CurrentContext)
}

// BindFlags is a convenience method to bind the specified flags to their associated variables
func BindContextFlags(contextInfo *clientcmdapi.Context, flags *pflag.FlagSet, flagNames ContextOverrideFlags) {
	bindStringFlag(flags, &contextInfo.Cluster, flagNames.ClusterName)
	bindStringFlag(flags, &contextInfo.AuthInfo, flagNames.AuthInfoName)
	bindStringFlag(flags, &contextInfo.Namespace, flagNames.Namespace)
}

func bindStringFlag(flags *pflag.FlagSet, target *string, flagInfo FlagInfo) {
	// you can't register a flag without a long name
	if len(flagInfo.LongName) > 0 {
		flags.StringVarP(target, flagInfo.LongName, flagInfo.ShortName, flagInfo.Default, flagInfo.Description)
	}
}

func bindBoolFlag(flags *pflag.FlagSet, target *bool, flagInfo FlagInfo) {
	// you can't register a flag without a long name
	if len(flagInfo.LongName) > 0 {
		// try to parse Default as a bool.  If it fails, assume false
		boolVal, err := strconv.ParseBool(flagInfo.Default)
		if err != nil {
			boolVal = false
		}

		flags.BoolVarP(target, flagInfo.LongName, flagInfo.ShortName, boolVal, flagInfo.Description)
	}
}
