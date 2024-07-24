package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/go-plugin"

	"github.com/aws/aws-msk-iam-sasl-signer-go/signer"
	"github.com/deviceinsight/kafkactl/v5/pkg/plugins/auth"
	"github.com/hashicorp/go-hclog"
)

var Version = "latest"
var BuildTime string
var GitCommit string

type tokenProvider struct {
	logger         hclog.Logger
	region         string
	role           string
	profile        string
	stsSessionName string
	token          string
}

// Init initializes the tokenProvider with the provided clientID and clientSecret.
// The provided tokenURL is used to perform the 2 legged client credentials flow.
func (t *tokenProvider) Init(options map[string]any, brokers []string) (err error) {

	t.logger.Debug("init", "options", options)

	if v, ok := options["debug"].(bool); ok && v {
		signer.AwsDebugCreds = v
	}

	if r, ok := options["region"].(string); ok {
		t.region = r
	}

	if r, ok := options["role"].(string); ok {
		t.role = r
	}

	if p, ok := options["profile"].(string); ok {
		t.profile = p
	}

	if n, ok := options["stsSessionName"].(string); ok {
		t.stsSessionName = n
	}

	t.logger.Debug("plugin initialized")
	return nil
}

// Token returns a new accessToken or an error as appropriate.
func (t *tokenProvider) Token() (string, error) {

	expired := true
	var err error

	if t.token != "" {
		expired, err = t.isTokenExpired()
		if err != nil {
			return "", fmt.Errorf("failed to check if token is expired: %w", err)
		}
	}

	if expired {
		switch {
		case t.role != "":
			t.logger.Debug("fetching new token", "region", t.region, "role", t.role, "stsSessionName", t.stsSessionName)
			t.token, _, err = signer.GenerateAuthTokenFromRole(context.Background(), t.region, t.role, t.stsSessionName)
		case t.profile != "":
			t.logger.Debug("fetching new token", "role", t.role, "profile", t.profile)
			t.token, _, err = signer.GenerateAuthTokenFromProfile(context.Background(), t.region, t.profile)
		default:
			t.logger.Debug("fetching new token", "region", t.region)
			t.token, _, err = signer.GenerateAuthToken(context.Background(), t.region)
		}

		if err != nil {
			return "", fmt.Errorf("failed to generate token: %w", err)
		}

		t.logger.Debug("fetched new token")
		return t.token, nil
	}

	t.logger.Debug("reusing cached token")
	return t.token, nil
}

// isTokenExpired checks if the token is expired by decoding the token and checking the 'X-Amz-Expires' param of the token.
// Inspired by https://github.com/aws/aws-msk-iam-sasl-signer-go/blob/896b3e826e770470727dc53d8154cbf148a07aad/signer/msk_auth_token_provider.go#L227-L251
func (t *tokenProvider) isTokenExpired() (bool, error) {

	b, err := base64.RawURLEncoding.DecodeString(t.token)
	if err != nil {
		return true, fmt.Errorf("failed to base64 decode the token: %w", err)
	}

	parsedURL, err := url.Parse(string(b))
	if err != nil {
		return true, fmt.Errorf("failed to parse the signed url: %w", err)
	}

	params := parsedURL.Query()
	d := params.Get("X-Amz-Date")
	if d == "" {
		return true, nil
	}
	
	signedAt, err := time.Parse("20060102T150405Z", d)
	if err != nil {
		return false, fmt.Errorf("failed to parse the 'X-Amz-Date' param from signed url: %w", err)
	}

	e := params.Get(signer.ExpiresQueryKey)
	if e == "" {
		return true, nil
	}
	expires, err := strconv.ParseInt(e, 10, 64)
	if err != nil {
		return true, fmt.Errorf("failed to parse the '%s' param from signed url: %w", signer.ExpiresQueryKey, err)
	}

	// expire the token a bit earlier, just to be safe
	expiresAt := signedAt.Add(time.Duration(expires) * time.Second).Add(time.Minute * 1)

	return expiresAt.Before(time.Now()), nil
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

	logger.Debug("aws plugin started", "version", Version, "buildTime", BuildTime, "gitCommit", GitCommit)

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: auth.TokenProviderPluginSpec.Handshake,
		Plugins: map[string]plugin.Plugin{
			auth.TokenProviderPluginSpec.InterfaceIdentifier: &auth.TokenProviderPlugin{Impl: tokenProvider},
		},
	})
}
