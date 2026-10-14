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
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Regression test for https://github.com/CiscoDevNet/terraform-provider-meraki/issues/291:
// the hand-maintained Create of meraki_wireless_zigbee builds its body with
// plan.withWriteOnly(config).toBody(...).
func TestWirelessZigbeeWithWriteOnly(t *testing.T) {
	plan := WirelessZigbee{
		Enabled:                  types.BoolValue(true),
		LockManagementPasswordWo: types.StringNull(),
	}
	config := WirelessZigbee{
		Enabled:                  types.BoolValue(true),
		LockManagementPasswordWo: types.StringValue("dummy-lock-password"),
	}

	body := plan.withWriteOnly(config).toBody(context.Background(), WirelessZigbee{})

	assertBodyValue(t, body, "lockManagement.password", "dummy-lock-password")
	assertBodyValue(t, body, "enabled", "true")
	if !plan.LockManagementPasswordWo.IsNull() {
		t.Error("withWriteOnly modified the plan it was called on")
	}
}
