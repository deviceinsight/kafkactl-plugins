package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/deviceinsight/kafkactl/pkg/plugins"
	"github.com/deviceinsight/kafkactl/pkg/plugins/auth"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/go-plugin"
)

type tokenProvider struct {
	logger        hclog.Logger
	tokenAudience string
	credential    *azidentity.DefaultAzureCredential
	token         azcore.AccessToken
}

// Init initializes the tokenProvider with the provided clientID and clientSecret.
// The provided tokenURL is used to perform the 2 legged client credentials flow.
func (t *tokenProvider) Init(options map[string]any, brokers []string) (err error) {

	t.logger.Debug("init", "options", options)

	if len(brokers) != 1 {
		return fmt.Errorf("expected exactly 1 broker. got %d", len(brokers))
	}

	brackets := regexp.MustCompile(`\\[|\\]`)

	bootstrapServer := brackets.ReplaceAllString(brokers[0], "")

	eventhubURL, err := url.Parse("https://" + bootstrapServer)
	if err != nil {
		return fmt.Errorf("unable to parse bootstrapServer: %w", err)
	}

	tenantId, ok := options["tenantid"].(string)
	if !ok {
		tenantId = ""
	}

	t.tokenAudience = fmt.Sprintf("%s://%s/.default", eventhubURL.Scheme, eventhubURL.Hostname())

	credential, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
		ClientOptions:              azcore.ClientOptions{},
		AdditionallyAllowedTenants: nil,
		DisableInstanceDiscovery:   false,
		TenantID:                   tenantId,
	})
	if err != nil {
		return fmt.Errorf("failed to get default credential: %w", err)
	}

	t.credential = credential

	t.logger.Debug("plugin initialized")
	return nil
}

// Token returns a new accessToken or an error as appropriate.
func (t *tokenProvider) Token() (string, error) {

	if t.isTokenExpired() {
		t.logger.Debug("fetching token", "audience", t.tokenAudience)
		options := policy.TokenRequestOptions{Scopes: []string{t.tokenAudience}}
		token, err := t.credential.GetToken(context.Background(), options)
		if err != nil {
			return "", fmt.Errorf("failed to acquire token: %w", err)
		}

		t.token = token
		t.logger.Debug("fetched new token")
	}

	return t.token.Token, nil
}

func (t *tokenProvider) isTokenExpired() bool {
	// expire the token a bit earlier, just to be safe
	oneMinuteInFuture := time.Now().Add(time.Minute * 1)
	return t.token.ExpiresOn.Before(oneMinuteInFuture)
}

func main() {

	logger := hclog.New(&hclog.LoggerOptions{
		Level:      hclog.Trace,
		Output:     os.Stderr,
		JSONFormat: true,
	})

	tokenProvider := &tokenProvider{
		logger: logger,
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: auth.TokenProviderPluginSpec.Handshake,
		Plugins: map[string]plugin.Plugin{
			plugins.GenericInterfaceIdentifier: &auth.TokenProviderPlugin{Impl: tokenProvider},
		},
	})
}
