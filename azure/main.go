package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/go-plugin"

	azlog "github.com/Azure/azure-sdk-for-go/sdk/azcore/log"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/deviceinsight/kafkactl/v5/pkg/plugins/auth"
	"github.com/hashicorp/go-hclog"
)

var Version = "latest"
var BuildTime string
var GitCommit string

type tokenProvider struct {
	logger         hclog.Logger
	tokenAudience  string
	credential     *azidentity.DefaultAzureCredential
	token          azcore.AccessToken
	loggingOptions policy.LogOptions
}

// Init initializes the tokenProvider with the provided clientID and clientSecret.
// The provided tokenURL is used to perform the 2 legged client credentials flow.
func (t *tokenProvider) Init(options map[string]any, brokers []string) (err error) {

	t.logger.Debug("init", "options", options)

	t.configureLogging(options)

	if len(brokers) != 1 {
		return fmt.Errorf("expected exactly 1 broker. got %d", len(brokers))
	}

	brackets := regexp.MustCompile(`\\[|\\]`)

	bootstrapServer := brackets.ReplaceAllString(brokers[0], "")

	eventhubURL, err := url.Parse("https://" + bootstrapServer)
	if err != nil {
		return fmt.Errorf("unable to parse bootstrapServer: %w", err)
	}

	tenantID, ok := options["tenant-id"].(string)
	if !ok {
		tenantID = ""
	}

	clientID, ok := options["client-id"].(string)
	if ok {
		_ = os.Setenv("AZURE_CLIENT_ID", clientID)
	}

	t.tokenAudience = fmt.Sprintf("%s://%s/.default", eventhubURL.Scheme, eventhubURL.Hostname())

	credential, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
		ClientOptions: azcore.ClientOptions{
			Logging: t.loggingOptions,
		},
		AdditionallyAllowedTenants: nil,
		DisableInstanceDiscovery:   false,
		TenantID:                   tenantID,
	})
	if err != nil {
		return fmt.Errorf("failed to get default credential: %w", err)
	}

	t.credential = credential

	t.logger.Debug("plugin initialized")
	return nil
}

func (t *tokenProvider) configureLogging(options map[string]any) {

	verbose, ok := options["verbose"].(bool)
	if !ok || !verbose {
		return
	}

	azEvents, ok := options["az-events"].(string)
	if !ok {
		return
	}

	t.logger.Debug("verbose enabled", "azEvents", azEvents)

	logBody, ok := options["log-body"].(bool)
	if ok && logBody {
		t.loggingOptions.IncludeBody = true
	}

	logHeaders, ok := options["log-headers"].(string)
	if ok {
		t.loggingOptions.AllowedHeaders = strings.Split(logHeaders, ",")
	}

	logQueryParams, ok := options["log-query-params"].(string)
	if ok {
		t.loggingOptions.AllowedQueryParams = strings.Split(logQueryParams, ",")
	}

	azlog.SetListener(func(event azlog.Event, message string) {
		t.logger.Debug(message, "event", event)
	})

	var events []azlog.Event

	for _, event := range strings.Split(azEvents, ",") {
		events = append(events, azlog.Event(event))
	}

	azlog.SetEvents(events...)
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

	logger.Debug("azure plugin started", "version", Version, "buildTime", BuildTime, "gitCommit", GitCommit)

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: auth.TokenProviderPluginSpec.Handshake,
		Plugins: map[string]plugin.Plugin{
			auth.TokenProviderPluginSpec.InterfaceIdentifier: &auth.TokenProviderPlugin{Impl: tokenProvider},
		},
	})
}
