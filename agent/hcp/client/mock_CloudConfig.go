package client

import (
	"crypto/tls"
	"errors"
	"net/url"

	hcpcfg "github.com/hashicorp/hcp-sdk-go/config"
	"github.com/hashicorp/hcp-sdk-go/profile"
	"golang.org/x/oauth2"
)

type mockCloudCfg struct{}

func (m *mockCloudCfg) Token() (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken: "test-token",
	}, nil
}

func (m *mockCloudCfg) APITLSConfig() *tls.Config     { return nil }
func (m *mockCloudCfg) SCADAAddress() string          { return "" }
func (m *mockCloudCfg) SCADATLSConfig() *tls.Config   { return &tls.Config{} }
func (m *mockCloudCfg) APIAddress() string            { return "" }
func (m *mockCloudCfg) PortalURL() *url.URL           { return &url.URL{} }
func (m *mockCloudCfg) Profile() *profile.UserProfile { return &profile.UserProfile{} }

type MockCloudCfg struct{}

func (m MockCloudCfg) HCPConfig(opts ...hcpcfg.HCPConfigOption) (hcpcfg.HCPConfig, error) {
	return &mockCloudCfg{}, nil
}

type MockErrCloudCfg struct{}

func (m MockErrCloudCfg) HCPConfig(opts ...hcpcfg.HCPConfigOption) (hcpcfg.HCPConfig, error) {
	return nil, errors.New("test bad HCP config")
}
