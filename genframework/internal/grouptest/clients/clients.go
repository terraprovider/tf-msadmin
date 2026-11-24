// Package clients mirrors the provider-side clients contract for the fixture.
package clients

import "github.com/terraprovider/tf-msadmin/genframework/internal/grouptest/cs"

type Client struct {
	CS       *cs.Service
	TenantID string
}
