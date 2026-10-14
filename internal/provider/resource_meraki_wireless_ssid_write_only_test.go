// Copyright © 2024 Cisco Systems, Inc. and its affiliates.
// All rights reserved.
//
// Licensed under the Mozilla Public License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://mozilla.org/MPL/2.0/
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Regression tests for https://github.com/CiscoDevNet/terraform-provider-meraki/issues/291:
// Terraform always sets write-only attributes to null in the plan, so building the request
// body from the plan alone never sent any `*_wo` value to the API.

func TestWirelessSSIDWithWriteOnly(t *testing.T) {
	plan := WirelessSSID{
		Name:  types.StringValue("planned-name"),
		PskWo: types.StringNull(),
		RadiusServers: []WirelessSSIDRadiusServers{
			{Host: types.StringValue("10.0.0.1"), Port: types.Int64Value(1812), SecretWo: types.StringNull()},
			{Host: types.StringValue("10.0.0.2"), Port: types.Int64Value(1813), SecretWo: types.StringNull()},
		},
	}
	config := WirelessSSID{
		Name:  types.StringValue("config-name"),
		PskWo: types.StringValue("dummy-psk"),
		RadiusServers: []WirelessSSIDRadiusServers{
			{Host: types.StringValue("10.0.0.1"), Port: types.Int64Value(1812), SecretWo: types.StringValue("dummy-secret-1")},
			{Host: types.StringValue("10.0.0.2"), Port: types.Int64Value(1813), SecretWo: types.StringValue("dummy-secret-2")},
		},
	}

	body := plan.withWriteOnly(config).toBody(context.Background(), WirelessSSID{})

	assertBodyValue(t, body, "psk", "dummy-psk")
	assertBodyValue(t, body, "radiusServers.0.secret", "dummy-secret-1")
	assertBodyValue(t, body, "radiusServers.1.secret", "dummy-secret-2")
	assertBodyValue(t, body, "radiusServers.0.host", "10.0.0.1")
	assertBodyValue(t, body, "radiusServers.1.port", "1813")
	// Ordinary attributes must still come from the plan.
	assertBodyValue(t, body, "name", "planned-name")

	// The plan itself must not be modified, it is later written to the state.
	if !plan.PskWo.IsNull() || !plan.RadiusServers[0].SecretWo.IsNull() || !plan.RadiusServers[1].SecretWo.IsNull() {
		t.Error("withWriteOnly modified the plan it was called on")
	}
}

// Terraform core rejects plans whose nested list length or element order differs from the config
// for non-computed attributes, so plan and config elements are matched by index. Each secret must
// end up on its own server, and a server without a write-only secret must not get one.
func TestWirelessSSIDWithWriteOnlyPerElementMapping(t *testing.T) {
	plan := WirelessSSID{RadiusServers: []WirelessSSIDRadiusServers{
		{Host: types.StringValue("10.0.0.1"), SecretWo: types.StringNull()},
		{Host: types.StringValue("10.0.0.2"), SecretWo: types.StringNull()},
		{Host: types.StringValue("10.0.0.3"), SecretWo: types.StringNull()},
	}}
	config := WirelessSSID{RadiusServers: []WirelessSSIDRadiusServers{
		{Host: types.StringValue("10.0.0.1"), SecretWo: types.StringValue("dummy-secret-a")},
		{Host: types.StringValue("10.0.0.2"), SecretWo: types.StringNull()},
		{Host: types.StringValue("10.0.0.3"), SecretWo: types.StringValue("dummy-secret-c")},
	}}

	body := plan.withWriteOnly(config).toBody(context.Background(), WirelessSSID{})

	assertBodyValue(t, body, "radiusServers.0.host", "10.0.0.1")
	assertBodyValue(t, body, "radiusServers.0.secret", "dummy-secret-a")
	assertBodyValue(t, body, "radiusServers.1.host", "10.0.0.2")
	if gjson.Get(body, "radiusServers.1.secret").Exists() {
		t.Error("radiusServers.1 must not get a secret")
	}
	assertBodyValue(t, body, "radiusServers.2.host", "10.0.0.3")
	assertBodyValue(t, body, "radiusServers.2.secret", "dummy-secret-c")
}

// When both an attribute and its `_wo` counterpart are set, the `_wo` value is sent. Without a
// `_wo` value the plain attribute is still sent, as before.
func TestWirelessSSIDWithWriteOnlyPrecedence(t *testing.T) {
	plan := WirelessSSID{
		Psk:           types.StringValue("dummy-plain-psk"),
		RadiusServers: []WirelessSSIDRadiusServers{{Secret: types.StringValue("dummy-plain-secret")}},
	}
	config := WirelessSSID{
		Psk:           types.StringValue("dummy-plain-psk"),
		PskWo:         types.StringValue("dummy-wo-psk"),
		RadiusServers: []WirelessSSIDRadiusServers{{Secret: types.StringValue("dummy-plain-secret"), SecretWo: types.StringValue("dummy-wo-secret")}},
	}

	body := plan.withWriteOnly(config).toBody(context.Background(), WirelessSSID{})
	assertBodyValue(t, body, "psk", "dummy-wo-psk")
	assertBodyValue(t, body, "radiusServers.0.secret", "dummy-wo-secret")

	body = plan.withWriteOnly(WirelessSSID{RadiusServers: []WirelessSSIDRadiusServers{{}}}).toBody(context.Background(), WirelessSSID{})
	assertBodyValue(t, body, "psk", "dummy-plain-psk")
	assertBodyValue(t, body, "radiusServers.0.secret", "dummy-plain-secret")
}

func TestWirelessSSIDWithWriteOnlyListLengthMismatch(t *testing.T) {
	plan := WirelessSSID{RadiusServers: []WirelessSSIDRadiusServers{{Host: types.StringValue("10.0.0.1")}}}
	config := WirelessSSID{}

	body := plan.withWriteOnly(config).toBody(context.Background(), WirelessSSID{})

	if gjson.Get(body, "radiusServers.0.secret").Exists() {
		t.Error("secret must not be set when plan and config lists do not match")
	}
	assertBodyValue(t, body, "radiusServers.0.host", "10.0.0.1")
}

// TestUnitMerakiWirelessSSID_writeOnly runs Terraform against a fake Meraki API and checks that
// write-only values reach the request body on create and update, and never reach the state or
// the plan, including after a failed update.
func TestUnitMerakiWirelessSSID_writeOnly(t *testing.T) {
	const (
		psk1    = "dummy-psk-291-first"
		psk2    = "dummy-psk-291-second"
		secret1 = "dummy-radius-secret-291-first"
		secret2 = "dummy-radius-secret-291-second"
	)
	secrets := []string{psk1, psk2, secret1, secret2}
	api := newFakeSSIDAPI()
	server := httptest.NewServer(api)
	defer server.Close()

	step := func(name, psk string, pskVersion int, radiusSecret string, radiusVersion int) resource.TestStep {
		return resource.TestStep{
			Config: testWriteOnlyConfig(server.URL, name),
			ConfigVariables: config.Variables{
				"psk":                   config.StringVariable(psk),
				"psk_version":           config.IntegerVariable(pskVersion),
				"radius_secret":         config.StringVariable(radiusSecret),
				"radius_secret_version": config.IntegerVariable(radiusVersion),
			},
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{noSecrets(secrets)},
			},
			ConfigStateChecks: []statecheck.StateCheck{
				noSecrets(secrets),
				statecheck.ExpectKnownValue("meraki_wireless_ssid.psk", tfjsonpath.New("psk_wo"), knownvalue.Null()),
				statecheck.ExpectKnownValue("meraki_wireless_ssid.psk", tfjsonpath.New("psk"), knownvalue.Null()),
				statecheck.ExpectKnownValue("meraki_wireless_ssid.psk", tfjsonpath.New("psk_wo_version"), knownvalue.Int64Exact(int64(pskVersion))),
				statecheck.ExpectKnownValue("meraki_wireless_ssid.radius", tfjsonpath.New("radius_servers").AtSliceIndex(0).AtMapKey("secret_wo"), knownvalue.Null()),
				statecheck.ExpectKnownValue("meraki_wireless_ssid.radius", tfjsonpath.New("radius_servers").AtSliceIndex(0).AtMapKey("host"), knownvalue.StringExact("10.0.0.1")),
			},
		}
	}

	createStep := step("ssid-a", psk1, 1, secret1, 1)
	createStep.Check = resource.ComposeTestCheckFunc(
		api.checkLastPut("0", "psk", psk1),
		api.checkLastPut("1", "radiusServers.0.secret", secret1),
		api.checkLastPut("1", "radiusServers.0.host", "10.0.0.1"),
		api.checkLastPut("1", "radiusServers.0.port", "1812"),
	)

	// Rotating the secret together with its version must send the new value.
	rotateStep := step("ssid-a", psk2, 2, secret1, 1)
	rotateStep.ConfigPlanChecks.PreApply = append(rotateStep.ConfigPlanChecks.PreApply,
		plancheck.ExpectResourceAction("meraki_wireless_ssid.psk", plancheck.ResourceActionUpdate))
	rotateStep.Check = api.checkLastPut("0", "psk", psk2)

	// Rotating a secret inside a nested list must send it for the right element.
	rotateNestedStep := step("ssid-a", psk2, 2, secret2, 2)
	rotateNestedStep.ConfigPlanChecks.PreApply = append(rotateNestedStep.ConfigPlanChecks.PreApply,
		plancheck.ExpectResourceAction("meraki_wireless_ssid.radius", plancheck.ResourceActionUpdate))
	rotateNestedStep.Check = resource.ComposeTestCheckFunc(
		api.checkLastPut("1", "radiusServers.0.secret", secret2),
		api.checkLastPut("1", "radiusServers.0.host", "10.0.0.1"),
	)

	// Changing an ordinary attribute keeps sending the configured secret instead of clearing it.
	renameStep := step("ssid-b", psk2, 2, secret2, 2)
	renameStep.Check = resource.ComposeTestCheckFunc(
		api.checkLastPut("0", "name", "ssid-b"),
		api.checkLastPut("0", "psk", psk2),
	)

	// A failed update must not leave secrets in the state, checked right after the failure.
	failStep := step(fakeSSIDFailName, psk2, 2, secret2, 2)
	failStep.ExpectError = regexp.MustCompile(`Failed to configure object`)
	failStep.ConfigStateChecks = nil
	afterFailStep := resource.TestStep{
		RefreshState: true,
		Check:        noSecrets(secrets).checkState,
		// The failed rename is still pending.
		ExpectNonEmptyPlan: true,
	}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{createStep, rotateStep, rotateNestedStep, renameStep, failStep, afterFailStep, renameStep},
	})
}

func testWriteOnlyConfig(baseUrl, name string) string {
	return fmt.Sprintf(`
variable "psk" {
  type      = string
  ephemeral = true
}

variable "psk_version" {
  type = number
}

variable "radius_secret" {
  type      = string
  ephemeral = true
}

variable "radius_secret_version" {
  type = number
}

provider "meraki" {
  api_key  = "dummy-api-key"
  base_url = %q
  retries  = 0
}

resource "meraki_wireless_ssid" "psk" {
  network_id          = "N_test"
  number              = "0"
  name                = %q
  auth_mode           = "psk"
  encryption_mode     = "wpa"
  wpa_encryption_mode = "WPA2 only"
  psk_wo              = var.psk
  psk_wo_version      = var.psk_version
}

resource "meraki_wireless_ssid" "radius" {
  network_id = "N_test"
  number     = "1"
  name       = "radius"
  auth_mode  = "8021x-radius"
  radius_servers = [{
    host              = "10.0.0.1"
    port              = 1812
    secret_wo         = var.radius_secret
    secret_wo_version = var.radius_secret_version
  }]
}
`, baseUrl, name)
}

func assertBodyValue(t *testing.T, body, path, want string) {
	t.Helper()
	if got := gjson.Get(body, path); !got.Exists() || got.String() != want {
		// Only report presence, the values may be secrets.
		t.Errorf("request body %q: exists=%v, matches expected value=%v", path, got.Exists(), got.String() == want)
	}
}

// fakeSSIDFailName makes the fake API reject the PUT, to exercise the error path.
const fakeSSIDFailName = "force-api-error"

// fakeSSIDAPI is a minimal in-memory stand-in for the wireless SSID endpoints. Like the real
// API it echoes `psk` on GET and never returns RADIUS secrets.
type fakeSSIDAPI struct {
	mu       sync.Mutex
	ssids    map[string]map[string]any
	lastPuts map[string]string
}

func newFakeSSIDAPI() *fakeSSIDAPI {
	return &fakeSSIDAPI{ssids: map[string]map[string]any{}, lastPuts: map[string]string{}}
}

func (f *fakeSSIDAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	number, ok := strings.CutPrefix(r.URL.Path, "/networks/N_test/wireless/ssids/")
	if !ok || number == "" || strings.Contains(number, "/") {
		http.NotFound(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	ssid := f.ssids[number]
	if ssid == nil {
		ssid = map[string]any{"number": number}
		f.ssids[number] = ssid
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		var update map[string]any
		if err := json.Unmarshal(body, &update); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if update["name"] == fakeSSIDFailName {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"errors":["forced error"]}`))
			return
		}
		f.lastPuts[number] = string(body)
		for k, v := range update {
			ssid[k] = v
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, _ := json.Marshal(ssid)
	for i := range gjson.GetBytes(res, "radiusServers").Array() {
		res, _ = sjson.DeleteBytes(res, fmt.Sprintf("radiusServers.%d.secret", i))
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

func (f *fakeSSIDAPI) checkLastPut(number, path, want string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		got := gjson.Get(f.lastPuts[number], path)
		if !got.Exists() || got.String() != want {
			return fmt.Errorf("last PUT to SSID %s: %q exists=%v, matches expected value=%v", number, path, got.Exists(), got.String() == want)
		}
		return nil
	}
}

// noSecrets is a plan and state check that fails if any of the given values appears in the
// plan or state JSON. It never prints the values.
type noSecrets []string

func (s noSecrets) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	resp.Error = s.find("plan", req.Plan)
}

func (s noSecrets) CheckState(_ context.Context, req statecheck.CheckStateRequest, resp *statecheck.CheckStateResponse) {
	resp.Error = s.find("state", req.State)
}

// checkState is the same check as a resource.TestCheckFunc, for steps without state checks.
func (s noSecrets) checkState(state *terraform.State) error {
	return s.find("state", state)
}

func (s noSecrets) find(what string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	for i, secret := range s {
		if strings.Contains(string(raw), secret) {
			return fmt.Errorf("%s contains test secret #%d", what, i)
		}
	}
	return nil
}
