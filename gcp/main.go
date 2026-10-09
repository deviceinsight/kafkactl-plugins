package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/deviceinsight/kafkactl/v5/pkg/plugins/auth"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var Version = "latest"
var BuildTime string
var GitCommit string

// gcpScopes are requested from Application Default Credentials. cloud-platform
// authorizes the Managed Kafka API; userinfo.email lets the tokeninfo endpoint
// report the principal for user credentials.
var gcpScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
}

// gcpTokenInfoURL resolves the principal of an access token when ADC itself
// does not name it (gcloud user credentials).
const gcpTokenInfoURL = "https://oauth2.googleapis.com/tokeninfo" //nolint:gosec // public endpoint, not a credential

// gcpTokenHeader is the fixed JWT header Google Managed Kafka expects.
const gcpTokenHeader = `{"typ":"JWT","alg":"GOOG_OAUTH2_TOKEN"}` //nolint:gosec // fixed JWT header, not a credential

// refreshBuffer is how long before expiry a cached token is replaced.
const refreshBuffer = 20 * time.Second

// tokenProvider produces OAUTHBEARER tokens for Google Cloud Managed Service
// for Apache Kafka. It is the Go equivalent of Java's
// com.google.cloud.hosted.kafka.auth.GcpLoginCallbackHandler: a JWT-shaped
// token whose signature segment is a Google OAuth2 access token obtained from
// Application Default Credentials (gcloud ADC login,
// GOOGLE_APPLICATION_CREDENTIALS, GKE Workload Identity, ...).
type tokenProvider struct {
	logger hclog.Logger
	mutex  sync.Mutex
	source oauth2.TokenSource
	// principal resolves the account email that becomes the token subject.
	principal func(ctx context.Context, tok *oauth2.Token) (string, error)
	now       func() time.Time

	email        string
	replaceAt    time.Time
	currentToken string
}

// Init locates Application Default Credentials up front, so a missing login
// fails before any broker connection.
// The optional option 'principal' sets the token subject explicitly.
func (t *tokenProvider) Init(options map[string]any, _ []string) error {

	t.logger.Debug("init", "options", options)

	var configured string
	if val, ok := options["principal"]; ok {
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("option 'principal' must be a string, got %T", val)
		}
		configured = s
	}

	creds, err := google.FindDefaultCredentials(context.Background(), gcpScopes...)
	if err != nil {
		return fmt.Errorf("failed to load Google Application Default Credentials "+
			"(run 'gcloud auth application-default login' or use Workload Identity): %w", err)
	}

	known := configured
	if known == "" {
		known = emailFromCredentialsJSON(creds.JSON)
	}
	t.source = creds.TokenSource
	t.principal = func(ctx context.Context, tok *oauth2.Token) (string, error) {
		return resolveGCPPrincipal(ctx, known, tok)
	}
	t.now = time.Now

	t.logger.Debug("plugin initialized")
	return nil
}

// Token returns a valid token, refreshing shortly before the underlying
// access token expires.
func (t *tokenProvider) Token() (string, error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.currentToken != "" && t.now().Before(t.replaceAt) {
		t.logger.Debug("reusing cached token")
		return t.currentToken, nil
	}
	if err := t.refresh(context.Background()); err != nil {
		return "", err
	}
	t.logger.Debug("fetched new token", "principal", t.email)
	return t.currentToken, nil
}

func (t *tokenProvider) refresh(ctx context.Context) error {
	tok, err := t.source.Token()
	if err != nil {
		return fmt.Errorf("failed to obtain Google access token: %w", err)
	}
	if t.email == "" {
		email, err := t.principal(ctx, tok)
		if err != nil {
			return fmt.Errorf("failed to resolve Google principal: %w", err)
		}
		t.email = email
	}
	now := t.now()
	expiry := tok.Expiry
	if expiry.IsZero() {
		expiry = now.Add(time.Hour)
	}
	t.currentToken = buildGCPToken(tok.AccessToken, t.email, expiry, now)
	t.replaceAt = expiry.Add(-refreshBuffer)
	return nil
}

// buildGCPToken assembles header.payload.accessToken, each segment base64url
// encoded without padding, exactly as GcpLoginCallbackHandler does.
func buildGCPToken(accessToken, email string, expiry, issuedAt time.Time) string {
	payload, _ := json.Marshal(struct {
		Exp int64  `json:"exp"`
		Iat int64  `json:"iat"`
		Iss string `json:"iss"`
		Sub string `json:"sub"`
	}{expiry.Unix(), issuedAt.Unix(), "Google", email})
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(gcpTokenHeader)) + "." +
		enc.EncodeToString(payload) + "." +
		enc.EncodeToString([]byte(accessToken))
}

// emailFromCredentialsJSON extracts the principal named by an ADC file: a
// service-account key's client_email, or the target of an impersonated
// service account. User credentials name no principal and return "".
func emailFromCredentialsJSON(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var f struct {
		ClientEmail      string `json:"client_email"`
		ImpersonationURL string `json:"service_account_impersonation_url"`
	}
	if json.Unmarshal(raw, &f) != nil {
		return ""
	}
	if f.ClientEmail != "" {
		return f.ClientEmail
	}
	// .../projects/-/serviceAccounts/<email>:generateAccessToken
	if i := strings.LastIndex(f.ImpersonationURL, "/serviceAccounts/"); i >= 0 {
		rest := f.ImpersonationURL[i+len("/serviceAccounts/"):]
		if j := strings.Index(rest, ":"); j >= 0 {
			return rest[:j]
		}
	}
	return ""
}

// resolveGCPPrincipal returns the email that becomes the token subject: the
// configured or credentials-file one, else the GCE/GKE metadata server's
// default account (Workload Identity), else the tokeninfo endpoint.
func resolveGCPPrincipal(ctx context.Context, known string, tok *oauth2.Token) (string, error) {
	if known != "" {
		return known, nil
	}
	if metadata.OnGCE() {
		if email, err := metadata.EmailWithContext(ctx, "default"); err == nil && email != "" {
			return email, nil
		}
	}
	return tokenInfoEmail(ctx, http.DefaultClient, gcpTokenInfoURL, tok.AccessToken)
}

// tokenInfoEmail asks Google's tokeninfo endpoint which account owns the
// access token. The token travels in the POST body, never the URL.
func tokenInfoEmail(ctx context.Context, client *http.Client, endpoint, accessToken string) (string, error) {
	form := url.Values{"access_token": {accessToken}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("tokeninfo request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tokeninfo returned HTTP %d", resp.StatusCode)
	}
	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("failed to decode tokeninfo response: %w", err)
	}
	if info.Email == "" {
		return "", errors.New("tokeninfo response has no email; ADC must include the userinfo.email scope " +
			"or set the 'principal' option")
	}
	return info.Email, nil
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

	logger.Debug("gcp plugin started", "version", Version, "buildTime", BuildTime, "gitCommit", GitCommit)

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: auth.TokenProviderPluginSpec.Handshake,
		Plugins: map[string]plugin.Plugin{
			auth.TokenProviderPluginSpec.InterfaceIdentifier: &auth.TokenProviderPlugin{Impl: tokenProvider},
		},
	})
}
