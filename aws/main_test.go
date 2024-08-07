package main

import (
	"encoding/base64"
	"testing"
)

//nolint:lll
func TestToken_IsTokenExpired(t *testing.T) {

	t.Parallel()

	type testcase struct {
		expired bool
		name    string
		token   string
	}

	tcs := []testcase{
		{
			name:    "expired",
			token:   base64.RawURLEncoding.EncodeToString([]byte("https://kafka.us-west-2.amazonaws.com/?Action=kafka-cluster:Connect&User-Agent=aws-msk-iam-sasl-signer-go/1.0.0/go1.22.2&X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=MOCK-ACCESS-KEY/19700101/us-west-2/kafka-cluster/aws4_request&X-Amz-Date=19700101T000000Z&X-Amz-Expires=900&X-Amz-Security-Token=MOCK-SESSION-TOKEN&X-Amz-Signature=1fa6dbb97390db03591f0f3af836d27b68e5fac024a126b688a2177eba2cd587&X-Amz-SignedHeaders=host%")),
			expired: true,
		},
		{
			name:    "valid",
			token:   base64.RawURLEncoding.EncodeToString([]byte("https://kafka.us-west-2.amazonaws.com/?Action=kafka-cluster:Connect&User-Agent=aws-msk-iam-sasl-signer-go/1.0.0/go1.22.2&X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=MOCK-ACCESS-KEY/19700101/us-west-2/kafka-cluster/aws4_request&X-Amz-Date=99991231T234459Z&X-Amz-Expires=900&X-Amz-Security-Token=MOCK-SESSION-TOKEN&X-Amz-Signature=1fa6dbb97390db03591f0f3af836d27b68e5fac024a126b688a2177eba2cd587&X-Amz-SignedHeaders=host%")),
			expired: false,
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := &tokenProvider{
				token: tc.token,
			}

			got, err := p.isTokenExpired()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.expired {
				t.Errorf("got: %v, want: %v", got, tc.expired)
			}
		})
	}
}
