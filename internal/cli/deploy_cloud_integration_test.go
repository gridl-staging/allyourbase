//go:build cloud_integration

package cli

import "testing"

func TestDigitalOceanCloudIntegrationStub(t *testing.T) {
	t.Skip("cloud integration stub: configure DIGITALOCEAN_ACCESS_TOKEN and deployment fixtures")
}

func TestRailwayCloudIntegrationStub(t *testing.T) {
	t.Skip("cloud integration stub: configure RAILWAY_API_TOKEN and deployment fixtures")
}
