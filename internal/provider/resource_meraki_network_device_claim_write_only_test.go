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
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Regression test for https://github.com/CiscoDevNet/terraform-provider-meraki/issues/291:
// the hand-maintained Update of meraki_network_device_claim builds the claim details from
// plan.withWriteOnly(config).DetailsByDevice, so each value_wo must stay with its device and detail.
func TestNetworkDeviceClaimWithWriteOnly(t *testing.T) {
	detail := func(name string, valueWo types.String) NetworkDeviceClaimDetailsByDeviceDetails {
		return NetworkDeviceClaimDetailsByDeviceDetails{Name: types.StringValue(name), ValueWo: valueWo}
	}
	plan := NetworkDeviceClaim{DetailsByDevice: []NetworkDeviceClaimDetailsByDevice{
		{Serial: types.StringValue("Q-AAAA"), Details: []NetworkDeviceClaimDetailsByDeviceDetails{
			detail("username", types.StringNull()),
			detail("password", types.StringNull()),
		}},
		{Serial: types.StringValue("Q-BBBB"), Details: []NetworkDeviceClaimDetailsByDeviceDetails{
			detail("password", types.StringNull()),
		}},
	}}
	config := NetworkDeviceClaim{DetailsByDevice: []NetworkDeviceClaimDetailsByDevice{
		{Serial: types.StringValue("Q-AAAA"), Details: []NetworkDeviceClaimDetailsByDeviceDetails{
			detail("username", types.StringNull()),
			detail("password", types.StringValue("dummy-password-a")),
		}},
		{Serial: types.StringValue("Q-BBBB"), Details: []NetworkDeviceClaimDetailsByDeviceDetails{
			detail("password", types.StringValue("dummy-password-b")),
		}},
	}}

	got := plan.withWriteOnly(config).DetailsByDevice

	if !got[0].Details[0].ValueWo.IsNull() {
		t.Error("Q-AAAA username must not get a value_wo")
	}
	if got[0].Details[1].ValueWo.ValueString() != "dummy-password-a" {
		t.Error("Q-AAAA password: value_wo not taken from config")
	}
	if got[1].Details[0].ValueWo.ValueString() != "dummy-password-b" {
		t.Error("Q-BBBB password: value_wo not taken from config")
	}
	if got[0].Serial.ValueString() != "Q-AAAA" || got[1].Serial.ValueString() != "Q-BBBB" {
		t.Error("serials must come from the plan in order")
	}
	if !plan.DetailsByDevice[0].Details[1].ValueWo.IsNull() || !plan.DetailsByDevice[1].Details[0].ValueWo.IsNull() {
		t.Error("withWriteOnly modified the plan it was called on")
	}
}
